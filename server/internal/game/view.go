// Package game은 시뮬레이션을 세션으로 실행하고 화면에 상태를 전달한다.
package game

import (
	"encoding/json"

	"serverway/internal/sim"
)

type jobView struct {
	ID    int  `json:"id"`
	Start int  `json:"s"`
	End   int  `json:"e"`
	Done  bool `json:"d,omitempty"`
}

type nodeView struct {
	ID       int       `json:"id"`
	Kind     sim.Kind  `json:"kind"`
	Index    int       `json:"index"`
	X        int       `json:"x"`
	Y        int       `json:"y"`
	Level    int       `json:"level"`
	Slots    int       `json:"slots"`
	ProcMs   int       `json:"procMs"`
	Limit    int       `json:"limit"`
	Queue    int       `json:"queue"`
	Jobs     []jobView `json:"jobs"`
	Out      int       `json:"out"` // 돌아갈 차선을 기다리는 응답
	Pending  int       `json:"pending"`
	Stuck    int       `json:"stuck"`
	Draining bool      `json:"draining"`
	Speed    int       `json:"speed"`
	State    string    `json:"state"` // ok, busy, full, slow, down
	Done     int       `json:"done"`
	Fail     [4]int    `json:"fail"`
	Spawned  int       `json:"spawned"`
	Rate     int       `json:"rate"`     // 클라이언트: 초당 요청 1000배
	NextRate int       `json:"nextRate"` // 30초 뒤 예상
	Upgrade  int       `json:"upgrade"`  // 다음 레벨 비용
	Next     *upView   `json:"next,omitempty"`
	Refund   int       `json:"refund"`
	Routed   bool      `json:"routed"` // 클라이언트: 완료까지 이어지는 길이 있는지
}

type upView struct {
	Slots  int `json:"slots"`
	ProcMs int `json:"procMs"`
	Limit  int `json:"limit"`
}

type linkView struct {
	ID       int    `json:"id"`
	From     int    `json:"from"`
	To       int    `json:"to"`
	Level    int    `json:"level"`
	Cap      int    `json:"cap"`
	Used     int    `json:"used"`
	Waiting  int    `json:"waiting"`
	Travel   int    `json:"travel"`
	Room     int    `json:"room"`
	Moving   [2]int `json:"moving"`
	Pending  int    `json:"pending"`
	Passed   int    `json:"passed"`
	Fail     [4]int `json:"fail"`
	State    string `json:"state"` // ok, busy, full(전송 한계), jam(앞이 막힘), blocked
	Draining bool   `json:"draining"`
	Upgrade  int    `json:"upgrade"`
	NextCap  int    `json:"nextCap"`
	Refund   int    `json:"refund"`
}

type faultView struct {
	ID       int    `json:"id"`
	Type     string `json:"type"`
	State    string `json:"state"`
	Node     int    `json:"node,omitempty"`
	Link     int    `json:"link,omitempty"`
	StartAt  int    `json:"startAt"`
	EndAt    int    `json:"endAt"`
	Percent  int    `json:"percent"`
	FailedIn int    `json:"failedIn"`
}

type upcoming struct {
	X  int `json:"x"`
	Y  int `json:"y"`
	At int `json:"at"`
}

// head는 매 프레임 보내는 작은 상태다.
type head struct {
	Tick      int         `json:"tick"`
	Score     int         `json:"score"`
	Health    int         `json:"health"`
	Budget    int         `json:"budget"`
	Paused    bool        `json:"paused"`
	Started   bool        `json:"started"`
	Speed     int         `json:"speed"`
	Over      string      `json:"over"`
	Phase     int         `json:"phase"`
	PhaseAt   int         `json:"phaseAt"`
	PhaseEnd  int         `json:"phaseEnd"`
	ApiCost   int         `json:"apiCost"`
	DbCost    int         `json:"dbCost"`
	Revenue   int         `json:"revenueNeed"`
	Rank      int         `json:"rank"`
	RankName  string      `json:"rankName"`
	NextRank  int         `json:"nextRank"`
	Surge     int         `json:"surge"`
	Servers   int         `json:"servers"`
	Spawned   int         `json:"spawned"`
	Completed int         `json:"completed"`
	Failures  [4]int      `json:"failures"`
	Active    int         `json:"active"`
	Reward    *sim.Reward `json:"reward"`
	Faults    []faultView `json:"faults"`
	Upcoming  []upcoming  `json:"upcoming"`
	Rps10     int         `json:"rps10"`
	P95       int         `json:"p95"`
	Area      sim.Area    `json:"area"`
	Cache     int         `json:"cache"`
	Deadline  int         `json:"deadline"`
	Why       string      `json:"pauseReason"`
}

type result struct {
	Over      string          `json:"over"`
	Seconds   int             `json:"seconds"`
	Score     int             `json:"score"`
	Completed int             `json:"completed"`
	Failures  [4]int          `json:"failures"`
	Health    int             `json:"health"`
	Spent     int             `json:"spent"`
	PeakWait  int             `json:"peakWait"`
	Apis      int             `json:"apis"`
	Links     int             `json:"links"`
	Faults    []sim.FaultStat `json:"faults"`
	Advice    string          `json:"advice"`
	Seed      uint64          `json:"seed"`
	P95       int             `json:"p95"`
	PeakRps   int             `json:"peakRps"`
	Rank      string          `json:"rank"`
	Sources   int             `json:"sources"`
	Dbs       int             `json:"dbs"`
}

func makeHead(s *sim.Sim, why string) head {
	h := head{Tick: s.Tick, Score: s.Score, Health: s.Health, Budget: s.Budget, Paused: s.Paused, Started: s.Started, Speed: s.Speed, Over: s.Over,
		Phase: s.Phase, Spawned: s.Spawned, Completed: s.Completed, Failures: s.Failures, Active: s.Active, Reward: s.Pending,
		Faults: faults(s), Upcoming: []upcoming{}, Rps10: s.RPS10(), P95: s.P95(), Area: s.Area(), Cache: s.Bonus.Cache, Deadline: s.Cfg.Request.DeadlineSec + s.Bonus.Deadline}
	if s.Paused {
		h.Why = why
	}
	h.PhaseAt, h.PhaseEnd, h.Revenue, h.Surge = s.PhaseAt, s.PhaseEnd, s.RevenueNeed(), s.Surge(s.Tick)
	h.ApiCost, h.DbCost = s.BuildCost(sim.API), s.BuildCost(sim.DB)
	h.Rank, h.RankName = s.Rank()
	if h.Rank+1 < len(s.Cfg.Ranks) {
		h.NextRank = s.Cfg.Ranks[h.Rank+1].At
	}
	for _, n := range s.Nodes {
		if n.Kind != sim.Source {
			h.Servers++
		}
	}
	for _, c := range s.Coming {
		h.Upcoming = append(h.Upcoming, upcoming{c.X, c.Y, c.At})
	}
	return h
}

func faults(s *sim.Sim) []faultView {
	out := []faultView{}
	for _, f := range s.Faults {
		if f.State != "notice" && f.State != "active" {
			continue
		}
		out = append(out, faultView{f.ID, f.Def.Type, f.State, f.Node, f.Link, f.StartAt, f.EndAt, f.Def.Percent, f.FailedIn})
	}
	return out
}

func makeNode(s *sim.Sim, n *sim.Node) nodeView {
	lv := s.ServerLevel(n)
	v := nodeView{ID: n.ID, Kind: n.Kind, Index: n.Index, X: n.X, Y: n.Y, Level: n.Level, Slots: lv.Slots, ProcMs: lv.ProcMs, Limit: lv.Queue,
		Queue: len(n.Queue), Out: len(n.Out), Pending: n.Pending, Jobs: []jobView{}, Stuck: n.Stuck, Draining: n.Draining, Speed: n.Speed, State: "ok", Done: n.Done, Fail: n.Fail, Spawned: n.Spawned}
	for _, j := range n.Jobs {
		// 장애로 멈춘 동안에는 끝나는 시각을 알 수 없으므로 멈춘 지점을 그대로 보낸다.
		end := j.Start + max(1, lv.ProcMs/sim.TickMs)
		if n.Speed > 0 && n.Speed < 100 {
			end = s.Tick + (j.Left+n.Speed-1)/n.Speed
		} else if n.Speed == 0 {
			end = 0
		}
		v.Jobs = append(v.Jobs, jobView{j.Req.ID, j.Start, end, j.Done})
	}
	switch {
	case n.Kind != sim.Source && n.Speed == 0:
		v.State = "down"
	case n.Kind != sim.Source && n.Speed < 100:
		v.State = "slow"
	case len(n.Queue)*10 >= lv.Queue*8:
		v.State = "full"
	case len(n.Queue)*10 >= lv.Queue*4:
		v.State = "busy"
	}
	if n.Kind == sim.Source {
		v.Rate, v.NextRate = s.Rate(n, s.Tick), s.Rate(n, s.Tick+30*sim.TPS)
		v.Routed = s.Routed(n)
		return v
	}
	v.Refund = n.Paid * s.Cfg.RefundPercent / 100
	x := s.NextLevel(n)
	v.Upgrade, v.Next = x.Cost, &upView{x.Slots, x.ProcMs, x.Queue}
	return v
}

func makeLink(s *sim.Sim, l *sim.Link) linkView {
	rate := s.LinkRate(l)
	v := linkView{ID: l.ID, From: l.From, To: l.To, Level: l.Level, Cap: rate, Used: max(l.Lanes[0].Used, l.Lanes[1].Used), Room: s.LinkRoom(l), Waiting: l.Waiting,
		Travel: l.Travel, Moving: [2]int{len(l.Lanes[0].Moving), len(l.Lanes[1].Moving)}, Pending: l.Pending, Passed: l.Passed, Fail: l.Fail, State: "ok", Draining: l.Draining, Refund: l.Paid * s.Cfg.RefundPercent / 100}
	switch {
	case l.Blocked:
		v.State = "blocked"
	case l.Jam && len(l.Lanes[0].Moving) > 2:
		v.State = "jam"
	case l.Waiting > 1 && v.Used >= rate-1:
		v.State = "full"
	case v.Used*10 >= rate*8:
		v.State = "busy"
	}
	x := s.Cfg.LinkLevel(l.Level + 1)
	v.Upgrade, v.NextCap = x.Cost, x.PerSec+s.Bonus.LinkRate
	return v
}

func makeResult(s *sim.Sim) *result {
	if s.Over == "" {
		return nil
	}
	r := &result{Over: s.Over, Seconds: s.Tick / sim.TPS, Score: s.Score, Completed: s.Completed, Failures: s.Failures, Health: s.Health,
		Spent: s.Spent, PeakWait: s.PeakWait, Links: len(s.Links), Faults: s.Stats, Seed: s.Seed, P95: s.P95(), PeakRps: s.PeakRPS}
	_, r.Rank = s.Rank()
	if r.Faults == nil {
		r.Faults = []sim.FaultStat{}
	}
	// 대기열이나 기한 때문에 실패한 곳. 링크 위에서 기한이 지난 것은 도착지가 막힌 탓으로 본다.
	var api, db, src int
	for _, n := range s.Nodes {
		over := n.Fail[sim.FailOverflow] + n.Fail[sim.FailDeadline]
		switch n.Kind {
		case sim.API:
			r.Apis++
			api += over
			db += n.Fail[sim.FailDeadline] / 2
		case sim.DB:
			r.Dbs++
			db += over
		default:
			r.Sources++
			src += over
		}
	}
	for _, l := range s.Links {
		over := l.Fail[sim.FailDeadline]
		if n := s.Node(l.To); n != nil && n.Kind == sim.DB {
			db += over
		} else {
			api += over
		}
	}
	f := s.Failures
	total := f[0] + f[1] + f[2] + f[3]
	switch {
	case s.Completed == 0 && total == 0:
		r.Advice = "아직 처리한 요청이 없습니다. 클라이언트를 서버에, 서버를 DB에 연결하고 시작해 보세요."
	case total == 0:
		r.Advice = "실패한 요청이 하나도 없었습니다. 완벽한 운영입니다."
	case f[sim.FailNoRoute]*2 >= total:
		r.Advice = "길이 없어 실패한 요청이 가장 많았습니다. 모든 클라이언트를 서버에, 모든 서버를 DB에 연결하세요."
	case f[sim.FailFault]*2 >= total:
		r.Advice = "장애로 실패한 요청이 가장 많았습니다. 예고가 뜨면 다른 서버를 키우고 우회 링크를 준비하세요."
	case db >= api && db >= src:
		r.Advice = "DB에서 가장 많이 막혔습니다. 서버를 늘리기 전에 DB를 업그레이드하거나 샤드를 추가하세요."
	case api >= src:
		r.Advice = "서버에서 가장 많이 막혔습니다. 서버를 업그레이드하거나 추가하고 클라이언트를 여러 서버에 연결하세요."
	default:
		r.Advice = "클라이언트에서 나가지 못한 요청이 많았습니다. 링크 용량을 늘리거나 링크를 하나 더 만드세요."
	}
	return r
}

// delta는 바뀐 노드와 링크만 골라낸다.
type delta struct {
	nodes, links map[int]string
}

func newDelta() *delta { return &delta{map[int]string{}, map[int]string{}} }

func (d *delta) diff(s *sim.Sim) (nodes, links []json.RawMessage, goneN, goneL []int) {
	seen := map[int]bool{}
	for _, n := range s.Nodes {
		seen[n.ID] = true
		b, _ := json.Marshal(makeNode(s, n))
		if d.nodes[n.ID] != string(b) {
			d.nodes[n.ID] = string(b)
			nodes = append(nodes, b)
		}
	}
	for id := range d.nodes {
		if !seen[id] {
			delete(d.nodes, id)
			goneN = append(goneN, id)
		}
	}
	seen = map[int]bool{}
	for _, l := range s.Links {
		seen[l.ID] = true
		b, _ := json.Marshal(makeLink(s, l))
		if d.links[l.ID] != string(b) {
			d.links[l.ID] = string(b)
			links = append(links, b)
		}
	}
	for id := range d.links {
		if !seen[id] {
			delete(d.links, id)
			goneL = append(goneL, id)
		}
	}
	return
}
