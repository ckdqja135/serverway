package sim

import (
	"math"
	"slices"
)

type Kind int

const (
	Source Kind = iota
	API
	DB
)

// 실패 사유
const (
	FailNoRoute = iota
	FailOverflow
	FailDeadline
	FailFault
	failKinds
)

type State string

const (
	Waiting    State = "waiting"    // 발생지나 서버의 대기열
	Moving     State = "moving"     // 링크 위
	Processing State = "processing" // 서버 슬롯
	Sending    State = "sending"    // 처리를 마치고 다음 링크를 기다림
	Completed  State = "completed"
	Failed     State = "failed"
)

// 처리 단계. 요청은 API와 DB를 거친 뒤 지나온 링크의 반대 차선으로 되돌아온다.
const (
	ToAPI      = iota // API 처리가 필요함
	ToDB              // DB 처리가 필요함
	BackToAPI         // 응답이 API로 돌아가는 중
	BackToUser        // 응답이 발생지로 돌아가는 중
	Finished
)

type Request struct {
	ID       int
	Stage    int
	State    State
	Source   int
	Born     int
	Deadline int
	Hops     int
	Depart   int
	Arrive   int
	Path     []int // 지나왔고 아직 되돌아가지 않은 링크
}

type Job struct {
	Req   *Request
	Start int
	Left  int // 남은 작업량. 정상 속도는 틱마다 100
	Done  bool
}

type Node struct {
	ID       int
	Kind     Kind
	Index    int // 같은 종류 안의 번호
	X, Y     int
	Level    int
	Paid     int
	Queue    []*Request
	Jobs     []*Job
	Out      []*Request // API를 거쳐 돌아가는 응답 중 차선을 기다리는 것
	Pending  int        // 이 노드를 거쳐 왕복 중인 요청 수
	Draining bool
	Speed    int // 처리 속도 퍼센트. 장애가 바꾼다
	Def      int // 발생지 설정 번호. -1이면 마라톤에서 새로 생긴 발생지
	Born     int
	acc      int
	next     int
	Spawned  int
	Done     int
	Fail     [failKinds]int
	Stuck    int // 처리를 마쳤지만 보내지 못하는 요청 수
}

// Lane은 한 방향 차선이다. 앞이 막히면 뒤 요청이 줄을 선다.
type Lane struct {
	Moving []*Request
	credit int
	ring   [TPS]int
	Used   int // 최근 1초 동안 들어온 요청 수
}

const (
	Fwd  = 0
	Back = 1
)

type Link struct {
	ID       int
	From, To int
	Level    int
	Paid     int
	Cells    int
	Travel   int
	Lanes    [2]Lane
	Pending  int // 이 링크로 나갔다가 아직 돌아오지 않은 요청 수
	Draining bool
	Blocked  bool
	Jam      bool // 도착 서버가 꽉 차서 맨 앞 요청이 들어가지 못함
	Passed   int
	Waiting  int // 이 링크로 나가려고 기다리는 요청 수
	Fail     [failKinds]int
}

type Fault struct {
	ID       int
	Def      FaultDef
	State    string // pending, notice, active, ended, canceled
	Node     int
	Link     int
	StartAt  int
	EndAt    int
	FailedIn int // 장애 중 실패 수
}

// FaultStat은 장애 종류별 누적 기록이다. 끝난 장애 자체는 최근 것만 남긴다.
type FaultStat struct {
	Type   string `json:"type"`
	Count  int    `json:"count"`
	Failed int    `json:"failed"`
}

// Coming은 나타날 예정인 발생지다.
type Coming struct {
	X, Y, At int
	Def      int
}

type Reward struct {
	Phase   int         `json:"phase"`
	Income  int         `json:"income"`
	Options []RewardDef `json:"options"`
}

// Event는 화면 표시용이다. 게임 계산에는 쓰지 않는다.
type Event struct {
	K    string `json:"k"`
	At   int    `json:"at"`
	Req  int    `json:"r,omitempty"`
	Node int    `json:"n,omitempty"`
	Link int    `json:"l,omitempty"`
	A    int    `json:"a,omitempty"`
	B    int    `json:"b,omitempty"`
	S    int    `json:"s,omitempty"`
	Text string `json:"t,omitempty"`
}

const maxEvents = 4096

type Sim struct {
	Cfg       *Config
	Seed      uint64
	Tick      int
	Paused    bool
	Speed     int
	Started   bool
	Over      string // "", down(서비스 중단), done(직접 마침)
	Score     int
	Health    int
	Budget    int
	Nodes     []*Node
	Links     []*Link
	Faults    []*Fault
	Stats     []FaultStat
	Coming    []Coming
	Pending   *Reward
	Phase     int
	PhaseAt   int // 이번 구간이 시작한 틱
	PhaseEnd  int
	PeakRPS   int
	Spawned   int
	Completed int
	Failures  [failKinds]int
	Active    int
	PeakWait  int
	Bonus     struct{ Queue, Deadline, LinkRate, APIOpt, DBIndex, Cache int }
	Events    []Event
	Log       []LogEntry
	Spent     int

	rng      uint64
	nextID   int
	nextReq  int
	gainLeft int
	revenue  int
	nodeIx   map[int]*Node
	linkIx   map[int]*Link
	outs     map[int][]*Link
	ins      map[int][]*Link
	faultID  int
	faultAt  int // 다음 마라톤 장애의 시작 틱
	faultN   int
	sourceAt int // 다음 마라톤 발생지가 나타나는 틱
	sourceN  int
	lastSeq  int64
	lats     [100]int // 최근 완료 요청의 왕복 틱 수
	latN     int
	perSec   [10]int // 최근 10초의 초별 완료 수
	acks     map[int64]Ack
}

func New(cfg *Config, seed uint64) *Sim {
	s := &Sim{Cfg: cfg, Seed: seed, rng: seed, Speed: 1, Paused: true, Health: cfg.Health.Max, Budget: cfg.InitialBudget, acks: map[int64]Ack{}}
	for i, d := range cfg.Sources {
		if d.AppearSec == 0 {
			s.addSource(i)
		}
	}
	s.addNode(API, cfg.StartAPI.X, cfg.StartAPI.Y)
	s.addNode(DB, cfg.DB0.X, cfg.DB0.Y)
	for _, f := range cfg.Faults {
		s.addFault(f)
	}
	s.faultAt, s.sourceAt = cfg.Endless.FaultFromSec*TPS, cfg.Endless.SourceFromSec*TPS
	s.PhaseEnd = cfg.PhaseSeconds * TPS
	return s
}

func (s *Sim) addFault(f FaultDef) {
	s.faultID++
	s.Faults = append(s.Faults, &Fault{ID: s.faultID, Def: f, State: "pending", StartAt: f.AtSec * TPS, EndAt: (f.AtSec + f.DurationSec) * TPS})
}

// reindex는 노드나 링크가 바뀔 때 조회용 색인을 다시 만든다. 순서는 만든 순서 그대로다.
func (s *Sim) reindex() {
	s.nodeIx, s.linkIx = make(map[int]*Node, len(s.Nodes)), make(map[int]*Link, len(s.Links))
	s.outs, s.ins = map[int][]*Link{}, map[int][]*Link{}
	for _, n := range s.Nodes {
		s.nodeIx[n.ID] = n
	}
	for _, l := range s.Links {
		s.linkIx[l.ID] = l
		s.outs[l.From] = append(s.outs[l.From], l)
		s.ins[l.To] = append(s.ins[l.To], l)
	}
}

// splitmix64
func (s *Sim) rand(n int) int {
	s.rng += 0x9e3779b97f4a7c15
	z := s.rng
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	return int(z % uint64(n))
}

func (s *Sim) emit(e Event) {
	e.At = s.Tick
	if len(s.Events) >= maxEvents {
		s.Events = s.Events[1:]
	}
	s.Events = append(s.Events, e)
}

// Take는 쌓인 표시용 이벤트를 꺼낸다.
func (s *Sim) Take() []Event {
	e := s.Events
	s.Events = nil
	return e
}

func (s *Sim) addNode(k Kind, x, y int) *Node {
	s.nextID++
	n := &Node{ID: s.nextID, Kind: k, X: x, Y: y, Speed: 100, Index: 1}
	for _, o := range s.Nodes {
		if o.Kind == k && o.Index >= n.Index {
			n.Index = o.Index + 1
		}
	}
	n.Born = s.Tick
	s.Nodes = append(s.Nodes, n)
	s.reindex()
	return n
}

func (s *Sim) addSource(def int) {
	d := s.Cfg.Sources[def]
	s.openSource(Coming{X: d.X, Y: d.Y, Def: def})
}

func (s *Sim) openSource(c Coming) {
	n := s.addNode(Source, c.X, c.Y)
	n.Def, n.next = c.Def, 1000
	s.emit(Event{K: "source", Node: n.ID})
}

func (s *Sim) Node(id int) *Node { return s.nodeIx[id] }
func (s *Sim) Link(id int) *Link { return s.linkIx[id] }

func (s *Sim) def(k Kind) ServerDef {
	if k == DB {
		return s.Cfg.DB
	}
	return s.Cfg.API
}

// BuildCost는 다음 한 대를 짓는 비용이다. 처음부터 있던 한 대 이후로 조금씩 오른다.
func (s *Sim) BuildCost(k Kind) int {
	d := s.def(k)
	return d.Cost + d.CostStep*max(0, s.count(k)-1)
}

// RevenueNeed는 예산 1을 버는 데 필요한 완료 수다.
func (s *Sim) RevenueNeed() int { return s.Cfg.RevenueEvery + s.count(API) + s.count(DB) }

func (s *Sim) count(k Kind) int {
	c := 0
	for _, n := range s.Nodes {
		if n.Kind == k && !n.Draining {
			c++
		}
	}
	return c
}

func (s *Sim) ServerLevel(n *Node) ServerLevel {
	if n.Kind == Source {
		return ServerLevel{Queue: s.Cfg.Source.Queue}
	}
	return s.boosted(n.Kind, s.def(n.Kind).Level(n.Level))
}

// NextLevel은 한 단계 올렸을 때의 값이다.
func (s *Sim) NextLevel(n *Node) ServerLevel {
	return s.boosted(n.Kind, s.def(n.Kind).Level(n.Level+1))
}

// boosted는 보상으로 얻은 개선을 반영한다.
func (s *Sim) boosted(k Kind, lv ServerLevel) ServerLevel {
	lv.Queue += s.Bonus.Queue
	opt := s.Bonus.APIOpt
	if k == DB {
		opt = s.Bonus.DBIndex
	}
	lv.ProcMs = lv.ProcMs * (100 - min(opt, 60)) / 100
	return lv
}

// LinkRoom은 한 차선에 동시에 올라갈 수 있는 요청 수다.
func (s *Sim) LinkRoom(l *Link) int { return l.Cells * s.Cfg.Link.RoomPerCell * (l.Level + 1) }

// Area는 지금 쓸 수 있는 보드 영역이다. 정해진 영역을 다 쓰면 사방으로 계속 넓어진다.
func (s *Sim) Area() Area {
	a := s.Cfg.Areas[0]
	for _, x := range s.Cfg.Areas {
		if s.Tick >= x.AtSec*TPS {
			a = x
		}
	}
	e := s.Cfg.Endless
	if e.ExpandEverySec > 0 && s.Tick >= e.ExpandFromSec*TPS {
		k := 1 + (s.Tick-e.ExpandFromSec*TPS)/(e.ExpandEverySec*TPS)
		w, h := max(a.W, min(e.MaxW, a.W+4*k)), max(a.H, min(e.MaxH, a.H+2*k))
		a.X, a.Y = a.X-(w-a.W)/2, a.Y-(h-a.H)/2
		a.W, a.H = w, h
	}
	return a
}

// Rank는 누적 완료 수로 정하는 성장 단계다.
func (s *Sim) Rank() (int, string) {
	i := 0
	for k, r := range s.Cfg.Ranks {
		if s.Completed >= r.At {
			i = k
		}
	}
	return i, s.Cfg.Ranks[i].Name
}

// P95는 최근 완료 요청의 왕복 시간(ms)이다.
func (s *Sim) P95() int {
	n := min(s.latN, len(s.lats))
	if n == 0 {
		return 0
	}
	v := slices.Clone(s.lats[:n])
	slices.Sort(v)
	return v[(n*95+99)/100-1] * TickMs
}

// RPS10은 최근 10초 동안 완료한 요청 수다.
func (s *Sim) RPS10() int {
	t := 0
	for _, v := range s.perSec {
		t += v
	}
	return t
}

func (s *Sim) LinkRate(l *Link) int { return s.Cfg.LinkLevel(l.Level).PerSec + s.Bonus.LinkRate }

// Surge는 오래 버틸수록 커지는 전체 트래픽 배율(퍼센트)이다.
func (s *Sim) Surge(tick int) int {
	e := s.Cfg.Endless
	t := tick - e.SurgeFromMin*60*TPS
	if e.SurgePerMin == 0 || t <= 0 {
		return 100
	}
	m := t * 100 / (60 * TPS) // 분의 100배
	return 100 + (e.SurgePerMin*m+m*m/200)/100
}

// Rate는 발생지의 현재 초당 요청 수(1000배)다.
func (s *Sim) Rate(n *Node, tick int) int { return s.baseRate(n, tick) * s.Surge(tick) / 100 }

// baseRate는 설정 지점 사이를 부드럽게 잇는다.
func (s *Sim) baseRate(n *Node, tick int) int {
	e := s.Cfg.Endless
	if n.Def < 0 {
		t := tick - n.Born
		ramp := min(t, e.RampMin*60*TPS)
		return e.RateStart + (e.RatePerMin*ramp+e.GrowthPerMin*(t-ramp))/(60*TPS)
	}
	pts := s.Cfg.Sources[n.Def].Rate
	for i := len(pts) - 1; i >= 0; i-- {
		a := pts[i]
		if tick < a[0]*TPS {
			continue
		}
		if i == len(pts)-1 {
			return a[1] + e.GrowthPerMin*(tick-a[0]*TPS)/(60*TPS)
		}
		b := pts[i+1]
		return a[1] + (b[1]-a[1])*(tick-a[0]*TPS)/((b[0]-a[0])*TPS)
	}
	return 0
}

func dist(ax, ay, bx, by int) float64 { return math.Hypot(float64(ax-bx), float64(ay-by)) }

// cells는 두 칸 사이 거리를 올림한 칸 수다.
func cells(ax, ay, bx, by int) int { return int(math.Ceil(dist(ax, ay, bx, by) - 1e-9)) }

func (s *Sim) LinkCost(a, b *Node) int {
	return s.Cfg.Link.BaseCost + s.Cfg.Link.PerCell*cells(a.X, a.Y, b.X, b.Y)
}

// Advance는 n틱까지 진행한다. 일시 정지, 보상 선택 대기, 종료 상태에서는 멈춘다.
func (s *Sim) Advance(n int) int {
	done := 0
	for ; done < n && !s.Paused && s.Pending == nil && s.Over == ""; done++ {
		s.step()
	}
	return done
}

func (s *Sim) step() {
	s.Tick++
	if s.Tick%TPS == 1 {
		s.gainLeft = s.Cfg.Health.MaxGainPerSec
		s.perSec[s.Tick/TPS%len(s.perSec)] = 0
	}
	s.stepGrowth()
	s.stepFaults()
	s.stepSources()
	s.stepArrivals()
	s.stepJobs()
	s.stepDispatch()
	s.stepStart()
	s.stepDeadlines()
	s.stepCleanup()
	s.stepEnd()
}

// stepGrowth는 발생지 예고와 등장, 마라톤 장애 예약을 처리한다.
func (s *Sim) stepGrowth() {
	for i, d := range s.Cfg.Sources {
		if d.AppearSec > 0 && s.Tick == (d.AppearSec-d.NoticeSec)*TPS {
			s.Coming = append(s.Coming, Coming{d.X, d.Y, d.AppearSec * TPS, i})
		}
	}
	e := s.Cfg.Endless
	if e.SourceEverySec > 0 && s.Tick == s.sourceAt-e.SourceNoticeSec*TPS {
		if x, y, ok := s.spot(); ok {
			s.Coming = append(s.Coming, Coming{x, y, s.sourceAt, -1})
		}
		s.sourceN++
		s.sourceAt += max(e.SourceMinEverySec, e.SourceEverySec-s.sourceN) * TPS
	}
	keep := s.Coming[:0]
	for _, c := range s.Coming {
		if c.At > s.Tick {
			keep = append(keep, c)
			continue
		}
		s.openSource(c)
	}
	s.Coming = keep

	if e.FaultEverySec > 0 && s.Tick == s.faultAt-30*TPS {
		types := []FaultDef{
			{Type: "api_slow", NoticeSec: 14, DurationSec: 18, Percent: 40},
			{Type: "link_block", NoticeSec: 14, DurationSec: 16},
			{Type: "api_down", NoticeSec: 16, DurationSec: 14},
		}
		d := types[s.rand(len(types))]
		d.AtSec = s.faultAt / TPS
		s.addFault(d)
		s.faultN++
		s.faultAt += max(e.FaultMinEverySec, e.FaultEverySec-s.faultN*2) * TPS
	}
	// 끝난 장애는 최근 것만 남긴다.
	if len(s.Faults) > 24 {
		for i, f := range s.Faults {
			if f.State == "ended" || f.State == "canceled" {
				s.Faults = append(s.Faults[:i:i], s.Faults[i+1:]...)
				break
			}
		}
	}
}

// spot은 새 발생지 자리를 고른다. 후보 중 다른 노드와 가장 멀리 떨어진 곳이다.
func (s *Sim) spot() (int, int, bool) {
	a := s.Area()
	bx, by, best := 0, 0, 0.0
	for range 32 {
		x, y := a.X+1+s.rand(max(1, a.W-2)), a.Y+1+s.rand(max(1, a.H-2))
		gap := math.Inf(1)
		for _, n := range s.Nodes {
			gap = math.Min(gap, dist(x, y, n.X, n.Y))
		}
		for _, c := range s.Coming {
			gap = math.Min(gap, dist(x, y, c.X, c.Y))
		}
		// 너무 외딴 곳보다 기존 서비스망 근처를 고른다.
		if gap >= 3 && gap <= 9 && gap > best {
			bx, by, best = x, y, gap
		}
	}
	return bx, by, best > 0
}

func (s *Sim) stat(t string) *FaultStat {
	for i := range s.Stats {
		if s.Stats[i].Type == t {
			return &s.Stats[i]
		}
	}
	s.Stats = append(s.Stats, FaultStat{Type: t})
	return &s.Stats[len(s.Stats)-1]
}

func (s *Sim) stepFaults() {
	for _, n := range s.Nodes {
		n.Speed = 100
	}
	for _, l := range s.Links {
		l.Blocked = false
	}
	for _, f := range s.Faults {
		switch f.State {
		case "pending":
			if s.Tick >= f.StartAt-f.Def.NoticeSec*TPS {
				s.noticeFault(f)
			}
		case "notice":
			if s.Tick >= f.StartAt {
				if !s.faultTargetAlive(f) {
					f.State = "canceled"
					s.emit(Event{K: "fault", A: f.ID, Text: "canceled"})
					break
				}
				f.State = "active"
				s.stat(f.Def.Type).Count++
				s.emit(Event{K: "fault", A: f.ID, Text: "active"})
			}
		case "active":
			if s.Tick >= f.EndAt || !s.faultTargetAlive(f) {
				f.State = "ended"
				s.emit(Event{K: "fault", A: f.ID, Text: "ended"})
			}
		}
		if f.State != "active" {
			continue
		}
		switch f.Def.Type {
		case "api_slow":
			s.Node(f.Node).Speed = f.Def.Percent
		case "api_down":
			s.Node(f.Node).Speed = 0
		case "link_block":
			s.Link(f.Link).Blocked = true
		}
	}
}

func (s *Sim) faultTargetAlive(f *Fault) bool {
	if f.Def.Type == "link_block" {
		l := s.Link(f.Link)
		return l != nil && !l.Draining
	}
	n := s.Node(f.Node)
	return n != nil && !n.Draining
}

// noticeFault는 많이 쓴 순서로 대상을 고른다. 정해진 초반 장애는 1위, 그 뒤로는 3위까지 중 하나다.
func (s *Sim) noticeFault(f *Fault) {
	type cand struct{ id, used int }
	var list []cand
	if f.Def.Type == "link_block" {
		for _, l := range s.Links {
			if !l.Draining && s.Node(l.From).Kind == Source {
				list = append(list, cand{l.ID, l.Passed})
			}
		}
	} else {
		for _, n := range s.Nodes {
			if n.Kind == API && !n.Draining {
				list = append(list, cand{n.ID, n.Done})
			}
		}
	}
	f.State = "canceled"
	if len(list) > 0 {
		slices.SortStableFunc(list, func(a, b cand) int { return b.used - a.used })
		top := 1
		if f.ID > len(s.Cfg.Faults) {
			top = min(3, len(list))
		}
		f.State = "notice"
		if id := list[s.rand(top)].id; f.Def.Type == "link_block" {
			f.Link = id
		} else {
			f.Node = id
		}
	}
	s.emit(Event{K: "fault", A: f.ID, Text: f.State})
}

func (s *Sim) stepSources() {
	if s.Tick < s.Cfg.Request.StartSec*TPS {
		return
	}
	for _, n := range s.Nodes {
		if n.Kind != Source {
			continue
		}
		n.acc += s.Rate(n, s.Tick) * TickMs / 1000
		for n.acc >= n.next {
			n.acc -= n.next
			n.next = 750 + s.rand(501)
			s.spawn(n)
		}
	}
}

func (s *Sim) spawn(n *Node) {
	s.nextReq++
	r := &Request{ID: s.nextReq, State: Waiting, Source: n.ID, Born: s.Tick, Deadline: s.Tick + (s.Cfg.Request.DeadlineSec+s.Bonus.Deadline)*TPS}
	s.Spawned++
	s.Active++
	n.Spawned++
	if len(n.Queue) >= s.Cfg.Source.Queue || s.Active > s.Cfg.Request.MaxActive {
		reason := FailOverflow
		if len(n.Queue) > 0 {
			if _, why := s.route(n, n.Queue[0]); why == FailNoRoute || why == FailFault {
				reason = why
			}
		}
		s.fail(r, reason, n, nil)
		return
	}
	n.Queue = append(n.Queue, r)
}

// stepArrivals는 차선 맨 앞 요청부터 들여보낸다. 서버가 꽉 차 있으면 링크 위에서 기다린다.
func (s *Sim) stepArrivals() {
	for _, l := range s.Links {
		l.Jam = false
		for d := range l.Lanes {
			ln := &l.Lanes[d]
			if len(ln.Moving) == 0 || ln.Moving[0].Arrive > s.Tick {
				continue
			}
			r := ln.Moving[0]
			if d == Fwd {
				n := s.Node(l.To)
				if n.Kind == stageKind(r.Stage) && len(n.Queue) >= s.ServerLevel(n).Queue {
					l.Jam = true
					continue
				}
			}
			ln.Moving[0] = nil
			ln.Moving = ln.Moving[1:]
			if d == Fwd {
				s.arrive(r, s.Node(l.To))
			} else {
				s.comeBack(r, l)
			}
		}
	}
}

// comeBack은 응답이 지나온 링크 하나를 되돌아온 것을 처리한다.
func (s *Sim) comeBack(r *Request, l *Link) {
	r.Path = r.Path[:len(r.Path)-1]
	l.Pending--
	n := s.Node(l.From)
	s.emit(Event{K: "in", Req: r.ID, Node: n.ID})
	if n.Kind == API {
		r.Stage, r.State = BackToUser, Sending
		n.Out = append(n.Out, r)
		return
	}
	s.complete(r, n)
}

func clearTail[T any](v []*T, from int) {
	for i := from; i < len(v); i++ {
		v[i] = nil
	}
}

func stageKind(stage int) Kind {
	if stage == ToAPI {
		return API
	}
	return DB
}

func (s *Sim) arrive(r *Request, n *Node) {
	if n.Kind != stageKind(r.Stage) || r.Hops > s.Cfg.Request.MaxHops {
		s.fail(r, FailNoRoute, n, nil)
		return
	}
	s.emit(Event{K: "in", Req: r.ID, Node: n.ID})
	r.State = Waiting
	n.Queue = append(n.Queue, r)
}

func (s *Sim) stepJobs() {
	for _, n := range s.Nodes {
		if n.Kind == Source {
			continue
		}
		keep := n.Jobs[:0]
		for _, j := range n.Jobs {
			if !j.Done {
				j.Left -= n.Speed
				if j.Left <= 0 {
					j.Done = true
					j.Req.Stage++
					j.Req.State = Sending
					n.Done++
					if n.Kind == API && s.Bonus.Cache > 0 && s.rand(100) < s.Bonus.Cache {
						// 캐시에 있으면 DB를 거치지 않고 바로 응답한다.
						j.Req.Stage = BackToUser
						n.Out = append(n.Out, j.Req)
						s.emit(Event{K: "cache", Req: j.Req.ID, Node: n.ID})
						continue
					}
				}
			}
			keep = append(keep, j)
		}
		clearTail(n.Jobs, len(keep))
		n.Jobs = keep
	}
}

func (s *Sim) complete(r *Request, n *Node) {
	r.State, r.Stage = Completed, Finished
	s.Active--
	s.Completed++
	n.Done++
	s.perSec[s.Tick/TPS%len(s.perSec)]++
	s.lats[s.latN%len(s.lats)] = s.Tick - r.Born
	s.latN++
	s.Score += s.Cfg.ScorePerComplete
	// 처리 수익. 서버가 많을수록 운영비가 들어 예산 1을 버는 데 더 많은 완료가 필요하다.
	if s.Cfg.RevenueEvery > 0 {
		if s.revenue++; s.revenue >= s.RevenueNeed() {
			s.revenue = 0
			s.Budget++
		}
	}
	gain := min(s.Cfg.Health.SuccessGain, s.gainLeft, s.Cfg.Health.Max-s.Health)
	s.Health += gain
	s.gainLeft -= gain
	s.emit(Event{K: "done", Req: r.ID, Node: n.ID, A: (s.Tick - r.Born) * TickMs})
}

func (s *Sim) fail(r *Request, reason int, n *Node, l *Link) {
	r.State = Failed
	s.Active--
	s.Failures[reason]++
	for _, id := range r.Path {
		if p := s.Link(id); p != nil {
			p.Pending--
		}
	}
	r.Path = nil
	e := Event{K: "fail", Req: r.ID, A: reason, B: (s.Tick - r.Born) * TickMs}
	if n != nil {
		n.Fail[reason]++
		e.Node = n.ID
	}
	if l != nil {
		l.Fail[reason]++
		e.Link = l.ID
	}
	penalty := s.Cfg.Health.FailPenalty
	for _, f := range s.Faults {
		if f.State == "active" {
			f.FailedIn++
			s.stat(f.Def.Type).Failed++
			penalty = s.Cfg.Health.FailPenalty * s.Cfg.Health.FaultPercent / 100
		}
	}
	s.Health -= penalty
	if s.Tick < s.Cfg.Health.LearnSeconds*TPS && s.Health < s.Cfg.Health.LearnFloor {
		s.Health = s.Cfg.Health.LearnFloor
	}
	s.emit(e)
}

// usable은 구조만 보고 다음 단계 이후까지 이어지는 경로인지 확인한다. faults가 참이면 장애도 반영한다.
func (s *Sim) usable(l *Link, stage int, faults bool) bool {
	if l.Draining || (faults && l.Blocked) {
		return false
	}
	to := s.Node(l.To)
	if to == nil || to.Draining || to.Kind != stageKind(stage) || (faults && to.Speed == 0) {
		return false
	}
	if stage >= ToDB {
		return true
	}
	for _, o := range s.outs[to.ID] {
		if s.usable(o, stage+1, faults) {
			return true
		}
	}
	return false
}

// eta는 이동 시간과 도착지에서 기다릴 시간을 더한 틱 수다.
func (s *Sim) eta(l *Link) int {
	to := s.Node(l.To)
	lv := s.ServerLevel(to)
	ahead := len(to.Queue) + len(to.Jobs)
	for _, o := range s.ins[to.ID] {
		ahead += len(o.Lanes[Fwd].Moving)
	}
	proc := lv.ProcMs * 100 / TickMs / max(to.Speed, 10)
	return l.Travel + ahead*proc/lv.Slots
}

const routeWait = -1

// route는 지금 보낼 링크를 고른다. 없으면 이유(경로 없음, 장애, 전송 대기)를 돌려준다.
func (s *Sim) route(n *Node, r *Request) (*Link, int) {
	var best, bestReady *Link
	bestEta, readyEta := 0, 0
	structural := false
	for _, l := range s.outs[n.ID] {
		if s.usable(l, r.Stage, false) {
			structural = true
		}
		if !s.usable(l, r.Stage, true) {
			continue
		}
		e := s.eta(l)
		if best == nil || e < bestEta {
			best, bestEta = l, e
		}
		if l.open(s, Fwd) && (bestReady == nil || e < readyEta) {
			bestReady, readyEta = l, e
		}
	}
	switch {
	case bestReady != nil:
		return bestReady, 0
	case best != nil:
		return best, routeWait
	case structural:
		return nil, FailFault
	}
	return nil, FailNoRoute
}

func (l *Link) open(s *Sim, d int) bool {
	return !l.Blocked && l.Lanes[d].credit >= 1000 && len(l.Lanes[d].Moving) < s.LinkRoom(l)
}

func (s *Sim) send(l *Link, d int, r *Request) {
	ln := &l.Lanes[d]
	ln.credit -= 1000
	ln.ring[s.Tick%TPS]++
	if d == Fwd {
		l.Passed++
		l.Pending++
		r.Path = append(r.Path, l.ID)
	}
	r.State, r.Depart, r.Arrive = Moving, s.Tick, s.Tick+l.Travel
	r.Hops++
	ln.Moving = append(ln.Moving, r)
	s.emit(Event{K: "go", Req: r.ID, Link: l.ID, A: r.Depart, B: r.Arrive, S: r.Stage})
}

func (s *Sim) backLink(r *Request) *Link { return s.Link(r.Path[len(r.Path)-1]) }

func (s *Sim) stepDispatch() {
	for _, l := range s.Links {
		per := s.LinkRate(l) * TickMs
		for d := range l.Lanes {
			ln := &l.Lanes[d]
			ln.credit = min(ln.credit+per, 1000+per)
			ln.ring[s.Tick%TPS] = 0
		}
		l.Waiting = 0
	}
	for _, n := range s.Nodes {
		if n.Kind == Source {
			for len(n.Queue) > 0 {
				l, why := s.route(n, n.Queue[0])
				if why == routeWait {
					l.Waiting += len(n.Queue)
				}
				if l == nil || why == routeWait {
					break
				}
				s.send(l, Fwd, n.Queue[0])
				n.Queue[0] = nil
				n.Queue = n.Queue[1:]
			}
			continue
		}
		// 응답을 먼저 내보낸다. 돌아가는 차선은 서버가 꽉 차도 막히지 않는다.
		keepOut := n.Out[:0]
		for _, r := range n.Out {
			if l := s.backLink(r); l.open(s, Back) {
				s.send(l, Back, r)
				continue
			}
			keepOut = append(keepOut, r)
		}
		clearTail(n.Out, len(keepOut))
		n.Out = keepOut

		n.Stuck = 0
		keep := n.Jobs[:0]
		for _, j := range n.Jobs {
			if j.Done {
				if n.Kind == DB {
					if l := s.backLink(j.Req); l.open(s, Back) {
						s.send(l, Back, j.Req)
						continue
					} else {
						l.Waiting++
					}
				} else {
					l, why := s.route(n, j.Req)
					if l != nil && why != routeWait {
						s.send(l, Fwd, j.Req)
						continue
					}
					if why == routeWait {
						l.Waiting++
					}
				}
				n.Stuck++
			}
			keep = append(keep, j)
		}
		clearTail(n.Jobs, len(keep))
		n.Jobs = keep
	}
	for _, l := range s.Links {
		for d := range l.Lanes {
			ln := &l.Lanes[d]
			ln.Used = 0
			for _, v := range ln.ring {
				ln.Used += v
			}
		}
	}
}

func (s *Sim) stepStart() {
	for _, n := range s.Nodes {
		if n.Kind == Source || n.Speed == 0 {
			continue
		}
		lv := s.ServerLevel(n)
		for len(n.Jobs) < lv.Slots && len(n.Queue) > 0 {
			r := n.Queue[0]
			n.Queue[0] = nil
			n.Queue = n.Queue[1:]
			r.State = Processing
			n.Jobs = append(n.Jobs, &Job{Req: r, Start: s.Tick, Left: lv.ProcMs * 100 / TickMs})
		}
	}
}

func (s *Sim) stepDeadlines() {
	for _, n := range s.Nodes {
		keep := n.Queue[:0]
		for _, r := range n.Queue {
			if r.Deadline > s.Tick {
				keep = append(keep, r)
				continue
			}
			reason := FailDeadline
			if n.Speed == 0 {
				reason = FailFault
			} else if n.Kind == Source {
				if _, why := s.route(n, r); why == FailNoRoute || why == FailFault {
					reason = why
				}
			}
			s.fail(r, reason, n, nil)
		}
		clearTail(n.Queue, len(keep))
		n.Queue = keep

		jobs := n.Jobs[:0]
		for _, j := range n.Jobs {
			if j.Req.Deadline > s.Tick {
				jobs = append(jobs, j)
				continue
			}
			reason := FailDeadline
			if n.Speed == 0 {
				reason = FailFault
			} else if j.Done && n.Kind == API {
				if _, why := s.route(n, j.Req); why == FailNoRoute || why == FailFault {
					reason = why
				}
			}
			s.fail(j.Req, reason, n, nil)
		}
		clearTail(n.Jobs, len(jobs))
		n.Jobs = jobs

		outs := n.Out[:0]
		for _, r := range n.Out {
			if r.Deadline > s.Tick {
				outs = append(outs, r)
				continue
			}
			reason := FailDeadline
			if s.backLink(r).Blocked {
				reason = FailFault
			}
			s.fail(r, reason, n, nil)
		}
		clearTail(n.Out, len(outs))
		n.Out = outs
	}
	for _, l := range s.Links {
		for d := range l.Lanes {
			ln := &l.Lanes[d]
			keep := ln.Moving[:0]
			for _, r := range ln.Moving {
				if r.Deadline > s.Tick {
					keep = append(keep, r)
					continue
				}
				reason := FailDeadline
				if d == Fwd && s.Node(l.To).Speed == 0 {
					reason = FailFault
				}
				s.fail(r, reason, nil, l)
			}
			clearTail(ln.Moving, len(keep))
			ln.Moving = keep
		}
	}
}

// stepCleanup은 왕복 중인 요청이 모두 돌아온 링크와 서버만 지운다.
func (s *Sim) stepCleanup() {
	for _, n := range s.Nodes {
		n.Pending = len(n.Queue) + len(n.Jobs) + len(n.Out)
	}
	for _, l := range s.Links {
		s.Node(l.To).Pending += l.Pending
	}
	for _, l := range s.Links {
		if n := s.Node(l.From); n.Draining && n.Pending == 0 {
			l.Draining = true
		}
	}
	before := len(s.Links) + len(s.Nodes)
	links := s.Links[:0]
	for _, l := range s.Links {
		if l.Draining && l.Pending == 0 {
			continue
		}
		links = append(links, l)
	}
	clearTail(s.Links, len(links))
	s.Links = links
	nodes := s.Nodes[:0]
	for _, n := range s.Nodes {
		if n.Draining && n.Pending == 0 && !s.linked(n.ID) {
			continue
		}
		nodes = append(nodes, n)
	}
	clearTail(s.Nodes, len(nodes))
	s.Nodes = nodes
	if len(s.Links)+len(s.Nodes) != before {
		s.reindex()
	}
}

func (s *Sim) linked(id int) bool {
	for _, l := range s.Links {
		if l.From == id || l.To == id {
			return true
		}
	}
	return false
}

func (s *Sim) stepEnd() {
	wait := 0
	for _, n := range s.Nodes {
		wait += len(n.Queue)
	}
	for _, l := range s.Links {
		if l.Jam {
			wait += len(l.Lanes[Fwd].Moving)
		}
	}
	s.PeakWait = max(s.PeakWait, wait)
	if s.Health <= 0 {
		s.Health = 0
		s.Over = "down"
		return
	}
	s.PeakRPS = max(s.PeakRPS, s.RPS10())
	if s.Tick < s.PhaseEnd {
		return
	}
	s.Phase++
	e, list := s.Cfg.Endless, s.Cfg.PhaseIncome
	income := list[min(s.Phase, len(list))-1] + max(0, s.Phase-len(list))*e.IncomeStep
	s.Budget += income
	s.PhaseAt = s.Tick
	s.PhaseEnd = s.Tick + min(max(e.PhaseMaxSec, s.Cfg.PhaseSeconds), s.Cfg.PhaseSeconds+s.Phase*e.PhaseStepSec)*TPS
	// 더 좋아질 수 없는 보상은 빼고 고른다.
	var pool []RewardDef
	for _, r := range s.Cfg.Rewards {
		if (r.ID == "cache" && s.Bonus.Cache >= 60) || (r.ID == "apiOpt" && s.Bonus.APIOpt >= 60) || (r.ID == "dbIndex" && s.Bonus.DBIndex >= 60) ||
			(r.ID == "deadline" && s.Bonus.Deadline >= 16) || (r.ID == "health" && s.Health >= s.Cfg.Health.Max) {
			continue
		}
		pool = append(pool, r)
	}
	a := s.rand(len(pool))
	b := (a + 1 + s.rand(len(pool)-1)) % len(pool)
	s.Pending = &Reward{Phase: s.Phase, Income: income, Options: []RewardDef{pool[a], pool[b]}}
}

// Routed는 발생지에서 완료까지 이어지는 길이 있는지 구조만 보고 답한다.
func (s *Sim) Routed(n *Node) bool {
	for _, l := range s.outs[n.ID] {
		if s.usable(l, ToAPI, false) {
			return true
		}
	}
	return false
}
