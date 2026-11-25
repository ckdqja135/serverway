package sim

import "fmt"

type Command struct {
	Seq   int64  `json:"seq"`
	Type  string `json:"type"`
	Kind  string `json:"kind,omitempty"` // build: api(기본), db, redis, scene: source, traffic, fault, reward
	X     int    `json:"x,omitempty"`
	Y     int    `json:"y,omitempty"`
	From  int    `json:"from,omitempty"`
	To    int    `json:"to,omitempty"`
	ID    int    `json:"id,omitempty"`
	Index int    `json:"index,omitempty"`
	Speed int    `json:"speed,omitempty"`
	On    bool   `json:"on,omitempty"`
}

type Ack struct {
	Seq   int64  `json:"seq"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Tick  int    `json:"tick"`
	ID    int    `json:"id,omitempty"` // 새로 만든 대상
}

type LogEntry struct {
	Tick int     `json:"tick"`
	Cmd  Command `json:"cmd"`
}

const (
	maxLog   = 50000
	maxLevel = 40
	keepAck  = 256
)

func (s *Sim) LastSeq() int64 { return s.lastSeq }

// Apply는 명령 하나를 검증하고 한 번에 반영한다. 같은 순번이 다시 오면 처음 결과를 그대로 돌려준다.
func (s *Sim) Apply(c Command) Ack {
	if c.Seq <= s.lastSeq {
		if a, ok := s.acks[c.Seq]; ok {
			return a
		}
		return Ack{Seq: c.Seq, Error: "이미 처리한 명령입니다", Tick: s.Tick}
	}
	s.lastSeq = c.Seq
	id, err := s.exec(c)
	a := Ack{Seq: c.Seq, OK: err == nil, Tick: s.Tick, ID: id}
	if err != nil {
		a.Error = err.Error()
	} else {
		s.Log = append(s.Log, LogEntry{s.Tick, c})
		if s.Cfg.Practice && s.Budget < practiceLow {
			s.Budget = practiceRefill
		}
	}
	s.acks[c.Seq] = a
	delete(s.acks, c.Seq-keepAck)
	return a
}

func (s *Sim) pay(cost int) error {
	if s.Budget < cost {
		return fmt.Errorf("예산이 부족합니다 (필요 %d, 보유 %d)", cost, s.Budget)
	}
	s.Budget -= cost
	s.Spent += cost
	return nil
}

func (s *Sim) exec(c Command) (int, error) {
	if s.Over != "" {
		return 0, fmt.Errorf("이미 끝난 게임입니다")
	}
	if len(s.Log) >= maxLog {
		return 0, fmt.Errorf("명령 기록 한도에 도달했습니다")
	}
	if s.Pending != nil && c.Type != "reward" && c.Type != "finish" && c.Type != "pause" && c.Type != "speed" {
		return 0, fmt.Errorf("먼저 보상을 선택하세요")
	}
	switch c.Type {
	case "pause":
		s.Paused = c.On
		if !c.On {
			s.Started = true
		}
		return 0, nil
	case "speed":
		if c.Speed < 1 || c.Speed > 3 {
			return 0, fmt.Errorf("속도는 1에서 3까지입니다")
		}
		s.Speed = c.Speed
		return 0, nil
	case "finish":
		s.Over = "done"
		return 0, nil
	case "build":
		k := API
		switch c.Kind {
		case "db":
			k = DB
		case "redis":
			k = Cache
		}
		return s.build(k, c.X, c.Y)
	case "link":
		return s.connect(c.From, c.To)
	case "upgradeNode":
		return c.ID, s.upgradeNode(c.ID)
	case "upgradeLink":
		return c.ID, s.upgradeLink(c.ID)
	case "storage":
		return c.ID, s.growStorage(c.ID)
	case "websocket":
		return c.ID, s.webSocket(c.ID)
	case "removeLink":
		return c.ID, s.removeLink(c.ID)
	case "removeNode":
		return c.ID, s.removeNode(c.ID)
	case "reward":
		return 0, s.reward(c.Index)
	case "scene":
		return 0, s.scene(c)
	}
	return 0, fmt.Errorf("알 수 없는 명령입니다")
}

// CanBuild는 서버를 놓을 수 있는 칸인지 확인한다. 나타날 예정인 클라이언트 자리도 비워 둔다.
func (s *Sim) CanBuild(x, y int) error {
	if a := s.Area(); x < a.X+1 || y < a.Y+1 || x > a.X+a.W-2 || y > a.Y+a.H-2 {
		return fmt.Errorf("아직 쓸 수 없는 자리입니다")
	}
	gap := float64(s.Cfg.MinNodeGap)
	for _, n := range s.Nodes {
		if dist(x, y, n.X, n.Y) < gap {
			return fmt.Errorf("다른 노드와 너무 가깝습니다")
		}
	}
	for _, d := range s.Cfg.Sources {
		if dist(x, y, d.X, d.Y) < gap {
			return fmt.Errorf("새 클라이언트가 들어올 자리입니다")
		}
	}
	for _, c := range s.Coming {
		if dist(x, y, c.X, c.Y) < gap {
			return fmt.Errorf("새 클라이언트가 들어올 자리입니다")
		}
	}
	return nil
}

func (s *Sim) build(k Kind, x, y int) (int, error) {
	if k == Cache && !s.Unlocked(s.Cfg.Unlock.Redis) {
		return 0, fmt.Errorf("아직 열리지 않았습니다")
	}
	if s.count(API)+s.count(DB)+s.count(Cache) >= s.Cfg.MaxServers {
		return 0, fmt.Errorf("모두 %d대까지 놓을 수 있습니다", s.Cfg.MaxServers)
	}
	if err := s.CanBuild(x, y); err != nil {
		return 0, err
	}
	cost := s.BuildCost(k)
	if err := s.pay(cost); err != nil {
		return 0, err
	}
	n := s.addNode(k, x, y)
	n.Paid = cost
	if k == DB {
		s.spread(0)
	}
	return n.ID, nil
}

func (s *Sim) CanLink(a, b *Node) error {
	switch {
	case a == nil || b == nil:
		return fmt.Errorf("연결할 대상이 없습니다")
	case a.ID == b.ID:
		return fmt.Errorf("자기 자신에게는 연결할 수 없습니다")
	case a.Draining || b.Draining:
		return fmt.Errorf("철거 중이라 연결할 수 없습니다")
	case a.Kind == Source && (b.Kind == DB || b.Kind == Cache):
		return fmt.Errorf("요청은 서버 처리를 먼저 거쳐야 합니다")
	case a.Kind == DB || a.Kind == Cache:
		return fmt.Errorf("연결은 서버에서 그립니다. 응답은 같은 링크로 돌아옵니다")
	case b.Kind == Source:
		return fmt.Errorf("연결은 클라이언트에서 서버 쪽으로 그립니다. 응답은 같은 링크로 돌아옵니다")
	case a.Kind == b.Kind:
		return fmt.Errorf("같은 종류끼리는 연결할 수 없습니다")
	}
	for _, l := range s.outs[a.ID] {
		if l.To == b.ID {
			if l.Draining {
				return fmt.Errorf("철거가 끝난 뒤 다시 연결할 수 있습니다")
			}
			return fmt.Errorf("이미 연결되어 있습니다")
		}
	}
	if len(s.Links) >= s.Cfg.MaxLinks {
		return fmt.Errorf("링크는 %d개까지 만들 수 있습니다", s.Cfg.MaxLinks)
	}
	return nil
}

func (s *Sim) connect(from, to int) (int, error) {
	a, b := s.Node(from), s.Node(to)
	if err := s.CanLink(a, b); err != nil {
		return 0, err
	}
	cost := s.LinkCost(a, b)
	if err := s.pay(cost); err != nil {
		return 0, err
	}
	s.nextID++
	n := cells(a.X, a.Y, b.X, b.Y)
	travel := max(1, (n*TPS+s.Cfg.Link.CellsPerSec-1)/s.Cfg.Link.CellsPerSec)
	s.Links = append(s.Links, &Link{ID: s.nextID, From: from, To: to, Paid: cost, Cells: n, Travel: travel})
	s.reindex()
	return s.nextID, nil
}

func (s *Sim) upgradeNode(id int) error {
	n := s.Node(id)
	if n == nil || n.Kind == Source {
		return fmt.Errorf("업그레이드할 수 없는 대상입니다")
	}
	if n.Draining {
		return fmt.Errorf("철거 중이라 업그레이드할 수 없습니다")
	}
	if n.Level+1 >= maxLevel {
		return fmt.Errorf("더 올릴 수 없습니다")
	}
	cost := s.def(n.Kind).Level(n.Level + 1).Cost
	if err := s.pay(cost); err != nil {
		return err
	}
	n.Level++
	n.Paid += cost
	return nil
}

func (s *Sim) upgradeLink(id int) error {
	l := s.Link(id)
	if l == nil || l.Draining {
		return fmt.Errorf("업그레이드할 수 없는 링크입니다")
	}
	if l.Level+1 >= maxLevel {
		return fmt.Errorf("더 올릴 수 없습니다")
	}
	cost := s.Cfg.LinkLevel(l.Level + 1).Cost
	if err := s.pay(cost); err != nil {
		return err
	}
	l.Level++
	l.Paid += cost
	return nil
}

// growStorage는 DB의 저장 공간을 한 단계 늘린다.
func (s *Sim) growStorage(id int) error {
	n := s.Node(id)
	if n == nil || n.Kind != DB || n.Draining || !s.Unlocked(s.Cfg.Unlock.Storage) || n.Store+1 >= maxLevel {
		return fmt.Errorf("저장 공간을 늘릴 수 없는 대상입니다")
	}
	cost := s.StorageCost(n)
	if err := s.pay(cost); err != nil {
		return err
	}
	n.Store++
	n.Paid += cost
	return nil
}

// webSocket은 링크를 WebSocket으로 바꾼다. 이미 지나가는 요청의 도착 시각은 그대로다.
func (s *Sim) webSocket(id int) error {
	l := s.Link(id)
	if l == nil || !s.CanWS(l) {
		return fmt.Errorf("WebSocket으로 바꿀 수 없는 링크입니다")
	}
	cost := s.WSCost()
	if err := s.pay(cost); err != nil {
		return err
	}
	l.Travel, l.WS = s.WSTravel(l), true
	l.Paid += cost
	return nil
}

// removeLink는 새 요청만 막는다. 이미 나간 요청은 응답까지 돌아온 뒤에 링크가 사라진다.
func (s *Sim) removeLink(id int) error {
	l := s.Link(id)
	if l == nil || l.Draining {
		return fmt.Errorf("철거할 수 없는 링크입니다")
	}
	l.Draining = true
	s.Budget += l.Paid * s.Cfg.RefundPercent / 100
	return nil
}

func (s *Sim) removeNode(id int) error {
	n := s.Node(id)
	if n == nil || n.Kind == Source || n.Draining {
		return fmt.Errorf("철거할 수 없는 대상입니다")
	}
	if n.Kind != Cache && s.count(n.Kind) <= 1 {
		return fmt.Errorf("마지막 한 대는 철거할 수 없습니다")
	}
	n.Draining = true
	s.Budget += n.Paid * s.Cfg.RefundPercent / 100
	for _, l := range s.Links {
		if l.To == n.ID && !l.Draining {
			l.Draining = true
			s.Budget += l.Paid * s.Cfg.RefundPercent / 100
		}
		if l.From == n.ID && !l.Draining {
			// 나가는 링크는 안에 남은 요청을 다 내보낼 때까지 쓰고, 환급은 지금 한 번만 한다.
			s.Budget += l.Paid * s.Cfg.RefundPercent / 100
			l.Paid = 0
		}
	}
	return nil
}

func (s *Sim) reward(i int) error {
	p := s.Pending
	if p == nil || i < 0 || i >= len(p.Options) || p.Taken[i] {
		return fmt.Errorf("선택할 보상이 없습니다")
	}
	s.grant(p.Options[i])
	p.Taken[i] = true
	if p.Left--; p.Left == 0 {
		s.Pending = nil
	}
	return nil
}

// WithReward는 보상을 받았다고 치고 f를 부른 뒤 되돌린다. 미리 보기에 쓴다.
func (s *Sim) WithReward(o RewardDef, f func()) {
	bonus, budget, health := s.Bonus, s.Budget, s.Health
	s.grant(o)
	f()
	s.Bonus, s.Budget, s.Health = bonus, budget, health
}

func (s *Sim) grant(o RewardDef) {
	switch o.ID {
	case "budget":
		s.Budget += o.Amount
	case "queue":
		s.Bonus.Queue += o.Amount
	case "deadline":
		s.Bonus.Deadline += o.Amount
	case "linkRate":
		s.Bonus.LinkRate += o.Amount
	case "apiOpt":
		s.Bonus.APIOpt += o.Amount
	case "dbIndex":
		s.Bonus.DBIndex += o.Amount
	case "cache":
		s.Bonus.Cache = min(60, s.Bonus.Cache+o.Amount)
	case "health":
		s.Health = min(s.Cfg.Health.Max, s.Health+o.Amount)
	case "storage":
		s.Bonus.Storage += o.Amount
	case "revenue":
		s.Bonus.Revenue += o.Amount
	}
}

// Replay는 시드와 명령 기록으로 같은 게임을 다시 계산한다. 일시 정지와 속도는 결과에 영향이 없어 건너뛴다.
func Replay(cfg *Config, seed uint64, log []LogEntry, until int) *Sim {
	s := New(cfg, seed)
	s.Paused = false
	for _, e := range log {
		s.Advance(e.Tick - s.Tick)
		if e.Cmd.Type != "pause" && e.Cmd.Type != "speed" {
			s.Apply(e.Cmd)
		}
	}
	s.Advance(until - s.Tick)
	return s
}
