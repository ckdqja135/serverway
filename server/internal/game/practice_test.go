package game

import (
	"testing"

	"serverway/internal/sim"
)

// 튜토리얼은 병목이 API, 링크, DB 순서로 옮겨 간다고 가르친다. 밸런스를 바꿔 이 순서가 깨지면 여기서 드러난다.
func TestPracticeLesson(t *testing.T) {
	s := sim.New(sim.Practice(sim.Default()), 7)
	seq := int64(0)
	do := func(c sim.Command) int {
		t.Helper()
		seq++
		c.Seq = seq
		a := s.Apply(c)
		if !a.OK {
			t.Fatalf("%+v: %s", c, a.Error)
		}
		return a.ID
	}
	of := func(k sim.Kind, i int) *sim.Node {
		for _, n := range s.Nodes {
			if n.Kind == k {
				if i == 0 {
					return n
				}
				i--
			}
		}
		t.Fatalf("노드가 없습니다: 종류 %d", k)
		return nil
	}
	until := func(what string, sec int, ok func() bool) {
		t.Helper()
		from := s.Tick
		for !ok() {
			if s.Tick-from > sec*sim.TPS {
				t.Fatalf("%s: %d초 안에 일어나지 않았습니다", what, sec)
			}
			if s.Advance(1) == 0 {
				t.Fatalf("%s: 시간이 멈췄습니다", what)
			}
		}
	}
	state := func(n *sim.Node) string { return makeNode(s, n).State }

	api, db := of(sim.API, 0), of(sim.DB, 0)
	do(sim.Command{Type: "link", From: of(sim.Source, 0).ID, To: api.ID})
	toDB := do(sim.Command{Type: "link", From: api.ID, To: db.ID})
	do(sim.Command{Type: "pause"})
	until("첫 왕복", 10, func() bool { return s.Completed >= 3 })
	do(sim.Command{Type: "scene", Kind: "source"})
	do(sim.Command{Type: "link", From: of(sim.Source, 1).ID, To: api.ID})

	do(sim.Command{Type: "scene", Kind: "traffic", Index: 2600})
	until("API 병목", 15, func() bool { return state(api) == "full" })
	if st := makeLink(s, s.Link(toDB)).State; st == "full" || state(db) == "full" {
		t.Fatalf("API보다 뒤가 먼저 막혔습니다: 링크 %s, DB %s", st, state(db))
	}
	do(sim.Command{Type: "upgradeNode", ID: api.ID})
	until("링크 병목", 12, func() bool { return makeLink(s, s.Link(toDB)).State == "full" })
	if state(db) == "full" {
		t.Fatal("링크보다 DB가 먼저 막혔습니다")
	}
	do(sim.Command{Type: "upgradeLink", ID: toDB})
	until("DB 병목", 15, func() bool { return state(db) == "full" })
	do(sim.Command{Type: "upgradeNode", ID: db.ID})
	until("막힘 해소", 30, func() bool { return state(api) == "ok" && state(db) == "ok" })

	do(sim.Command{Type: "scene", Kind: "traffic", Index: 1800})
	do(sim.Command{Type: "scene", Kind: "fault"})
	until("장애 예고", 3, func() bool { return len(faults(s)) > 0 })
	extra := do(sim.Command{Type: "build", X: 10, Y: 5})
	for i := range 2 {
		do(sim.Command{Type: "link", From: of(sim.Source, i).ID, To: extra})
	}
	do(sim.Command{Type: "link", From: extra, To: db.ID})
	until("장애 시작", 10, func() bool { return faults(s)[0].State == "active" })
	before := s.Completed
	until("장애 끝", 12, func() bool { return len(faults(s)) == 0 })
	if s.Completed-before < 10 {
		t.Fatalf("장애 중 우회 처리 %d건", s.Completed-before)
	}
	do(sim.Command{Type: "scene", Kind: "reward"})
	s.Advance(5)
	if s.Pending == nil {
		t.Fatal("보상이 나오지 않았습니다")
	}
	do(sim.Command{Type: "reward"})
	if s.Over != "" || s.Budget < 100 {
		t.Fatalf("over=%q budget=%d", s.Over, s.Budget)
	}
}

func TestSceneOnlyInPractice(t *testing.T) {
	s := sim.New(sim.Default(), 7)
	if a := s.Apply(sim.Command{Seq: 1, Type: "scene", Kind: "reward"}); a.OK {
		t.Fatal("본 게임에서 장면 명령이 통했습니다")
	}
}
