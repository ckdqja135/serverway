package sim

import "sort"

// Bot은 밸런스 확인과 테스트에 쓰는 자동 플레이어다. 사람과 같은 명령만 쓴다.
type Bot struct {
	Link, API, DB, LinkUp, Reroute, Prepare bool

	seq int64
}

func (b *Bot) do(s *Sim, c Command) bool {
	b.seq++
	c.Seq = b.seq
	return s.Apply(c).OK
}

func servers(s *Sim, k Kind) []*Node {
	var out []*Node
	for _, n := range s.Nodes {
		if n.Kind == k && !n.Draining {
			out = append(out, n)
		}
	}
	return out
}

// nearest는 가까운 순서로 정렬한 사본이다.
func nearest(list []*Node, x, y int) []*Node {
	out := append([]*Node(nil), list...)
	sort.SliceStable(out, func(i, j int) bool { return dist(out[i].X, out[i].Y, x, y) < dist(out[j].X, out[j].Y, x, y) })
	return out
}

// Act는 게임 시간 1초마다 부른다.
func (b *Bot) Act(s *Sim) {
	if s.Pending != nil {
		best, at := 99, 0
		for i, o := range s.Pending.Options {
			for rank, id := range []string{"dbIndex", "apiOpt", "cache", "linkRate", "budget", "health", "queue", "deadline"} {
				if o.ID == id && rank < best {
					best, at = rank, i
				}
			}
		}
		b.do(s, Command{Type: "reward", Index: at})
	}
	if !b.Link {
		return
	}
	apis, dbs := servers(s, API), servers(s, DB)
	has := func(from, to int) bool {
		for _, l := range s.outs[from] {
			if l.To == to {
				return true
			}
		}
		return false
	}
	connect := func(n *Node, to []*Node, want int) {
		for _, t := range nearest(to, n.X, n.Y) {
			if len(s.outs[n.ID]) >= want {
				return
			}
			if !has(n.ID, t.ID) {
				b.do(s, Command{Type: "link", From: n.ID, To: t.ID})
			}
		}
	}
	for _, a := range apis {
		connect(a, dbs, 1)
	}
	for _, n := range servers(s, Source) {
		want := 1
		if b.Reroute && len(apis) > 1 && s.Tick > 60*TPS {
			want = 2
		}
		connect(n, apis, want)
	}
	// 가장 뒤쪽 병목부터 해결한다. 예산이 모자라면 다른 곳에 쓰지 않고 모은다.
	if b.DB {
		for _, d := range dbs {
			if len(d.Queue)*3 < s.ServerLevel(d).Queue {
				continue
			}
			up := s.Cfg.DB.Level(d.Level + 1).Cost
			if build := s.BuildCost(DB); d.Level >= 4 && build < up {
				if id, ok := b.place(s, "db", d.X, d.Y); ok {
					// 가장 가까운 API 몇 대를 새 샤드에도 잇는다.
					for _, a := range nearest(apis, s.Node(id).X, s.Node(id).Y)[:min(3, len(apis))] {
						b.do(s, Command{Type: "link", From: a.ID, To: id})
					}
				}
				return
			}
			if !b.do(s, Command{Type: "upgradeNode", ID: d.ID}) {
				return
			}
		}
	}
	if b.LinkUp {
		for _, l := range s.Links {
			if l.Waiting >= 2 && !l.Jam && !l.Draining {
				b.do(s, Command{Type: "upgradeLink", ID: l.ID})
			}
		}
	}
	if b.Prepare {
		for _, f := range s.Faults {
			if f.State != "notice" || f.Def.Type == "link_block" {
				continue
			}
			// 멈출 API를 뺀 이웃을 키워 둔다.
			t := s.Node(f.Node)
			if t == nil {
				continue
			}
			for _, a := range nearest(apis, t.X, t.Y)[:min(3, len(apis))] {
				if a.ID != f.Node && a.Level < 2 && b.do(s, Command{Type: "upgradeNode", ID: a.ID}) {
					return
				}
			}
			if len(apis) < 3 {
				b.place(s, "api", t.X, t.Y)
			}
		}
	}
	if b.API {
		for _, a := range apis {
			if len(a.Queue)*3 < s.ServerLevel(a).Queue*2 || a.Stuck > 0 {
				continue
			}
			if up := s.Cfg.API.Level(a.Level + 1).Cost; up <= s.BuildCost(API)+20 && b.do(s, Command{Type: "upgradeNode", ID: a.ID}) {
				continue
			}
			if id, ok := b.place(s, "api", a.X, a.Y); ok {
				// 이 API로 가장 많이 보내는 클라이언트를 새 API에도 잇는다.
				var busy *Link
				for _, l := range s.ins[a.ID] {
					if busy == nil || l.Used() > busy.Used() {
						busy = l
					}
				}
				if busy != nil {
					b.do(s, Command{Type: "link", From: busy.From, To: id})
				}
				return
			}
		}
	}
}

// Used는 가는 차선이 최근 1초 동안 보낸 수다.
func (l *Link) Used() int { return l.Lanes[Fwd].Used }

// place는 기준점에서 가까운 빈 칸에 짓는다.
func (b *Bot) place(s *Sim, kind string, x, y int) (int, bool) {
	k := API
	if kind == "db" {
		k = DB
	}
	if s.Budget < s.BuildCost(k) {
		return 0, false
	}
	for r := 2; r < 14; r++ {
		for dx := -r; dx <= r; dx++ {
			for _, dy := range []int{-r, r, r - 1, 1 - r} {
				if s.CanBuild(x+dx, y+dy) == nil && b.do(s, Command{Type: "build", Kind: kind, X: x + dx, Y: y + dy}) {
					return s.Nodes[len(s.Nodes)-1].ID, true
				}
			}
		}
	}
	return 0, false
}

// Play는 봇으로 끝날 때까지, 길어도 maxSec초까지 진행한다.
func Play(cfg *Config, seed uint64, b *Bot, maxSec int) *Sim {
	s := New(cfg, seed)
	b.do(s, Command{Type: "pause", On: false})
	for s.Over == "" && s.Tick < maxSec*TPS {
		b.Act(s)
		s.Advance(TPS)
		s.Take()
	}
	return s
}
