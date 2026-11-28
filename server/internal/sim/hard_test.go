package sim

import "testing"

// hard는 하드 모드 튜토리얼 판이다. 시간이 저절로 일으키는 일이 없어 보안 사고 하나씩을 따로 볼 수 있다.
func hard(t *testing.T, load int) (*Sim, func(Command) Ack) {
	t.Helper()
	s := New(HardPractice(Default()), 3)
	seq := int64(0)
	do := func(c Command) Ack { seq++; c.Seq = seq; return s.Apply(c) }
	do(Command{Type: "pause"})
	if a := do(Command{Type: "scene", Kind: "traffic", Index: load}); !a.OK {
		t.Fatal(a.Error)
	}
	return s, do
}

// strikeNow는 보안 사고를 일으키고 공격하는 순간까지 진행한다. 예고 중에 할 일은 before에서 한다.
func strikeNow(t *testing.T, s *Sim, do func(Command) Ack, kind string, before func(*Fault)) *Fault {
	t.Helper()
	if a := do(Command{Type: "scene", Kind: kind}); !a.OK {
		t.Fatal(a.Error)
	}
	f := s.Faults[len(s.Faults)-1]
	for f.State == "pending" {
		s.Advance(1)
	}
	if f.State != "notice" || f.Node == 0 {
		t.Fatalf("%s 예고가 대상을 정하지 못함: %+v", kind, f)
	}
	if before != nil {
		before(f)
	}
	s.Advance(f.StartAt - s.Tick)
	return f
}

func TestHardStartsBuilt(t *testing.T) {
	s := New(Hard(Default()), 1)
	if s.count(API) != 3 || s.count(DB) != 2 || len(s.Links) != 7 || s.Budget != 500 {
		t.Fatalf("서버 %d, DB %d, 링크 %d, 예산 %d", s.count(API), s.count(DB), len(s.Links), s.Budget)
	}
	sources := 0
	for _, n := range s.Nodes {
		if n.Kind == Source {
			sources++
			if !s.Routed(n) {
				t.Fatalf("클라이언트 %d가 연결되지 않음", n.ID)
			}
		}
		if n.Paid != 0 {
			t.Fatal("처음부터 있던 서버는 철거해도 돌려받지 않아야 합니다")
		}
	}
	if _, name := s.Rank(); sources != 3 || name != "스타트업" {
		t.Fatalf("클라이언트 %d, 단계 %s", sources, name)
	}
	if s.Cfg.Health.LearnSeconds != 0 {
		t.Fatal("하드 모드에는 보호 시간이 없습니다")
	}
	// 보안 사고는 장애처럼 미리 예고되어 저절로 일어난다.
	b := Bot{Link: true}
	s.Paused = false
	for s.Tick < (s.Cfg.HardMode.Security.FromSec+1)*TPS && s.Over == "" {
		b.Act(s)
		s.Advance(TPS)
	}
	if len(s.Stats) == 0 || !security(s.Stats[0].Type) {
		t.Fatalf("보안 사고가 일어나지 않음: %+v", s.Stats)
	}
}

// 일반 모드는 보안 사고를 일으킬 수 없다.
func TestSecurityOnlyInHard(t *testing.T) {
	s := New(Practice(Default()), 1)
	if a := s.Apply(Command{Seq: 1, Type: "scene", Kind: "hack"}); a.OK {
		t.Fatal("일반 튜토리얼에서 보안 사고가 일어남")
	}
	if a := s.Apply(Command{Seq: 2, Type: "patch", ID: 2}); a.OK {
		t.Fatal("예고가 없는데 패치됨")
	}
}

func TestHackStealsUnlessPatched(t *testing.T) {
	s, do := hard(t, 1000)
	f := strikeNow(t, s, do, "hack", nil)
	if had := s.Budget + f.Value; f.State != "ended" || f.Value == 0 || f.Value != had*30/100 {
		t.Fatalf("해킹 뒤 예산 %d, %+v", s.Budget, f)
	}
	if s.Node(f.Node).Kind != API {
		t.Fatal("해킹은 서버를 노립니다")
	}

	before, cost := s.Budget, s.PatchCost()
	f = strikeNow(t, s, do, "hack", func(f *Fault) {
		if a := do(Command{Type: "patch", ID: f.Node}); !a.OK {
			t.Fatal(a.Error)
		}
		if a := do(Command{Type: "patch", ID: f.Node}); a.OK {
			t.Fatal("같은 예고에 두 번 패치됨")
		}
		s.Advance(1)
		if s.Node(f.Node).Speed != 0 {
			t.Fatal("패치하는 동안 서버가 멈춰야 합니다")
		}
	})
	if f.State != "blocked" || f.Value != 0 || s.Budget < before-cost {
		t.Fatalf("패치했는데 %s, 예산 %d (처음 %d, 패치 %d)", f.State, s.Budget, before, cost)
	}
	st := s.stat("hack")
	if st.Count != 2 || st.Blocked != 1 || st.Lost == 0 {
		t.Fatalf("기록 %+v", st)
	}
	s.Advance(s.Cfg.HardMode.Security.PatchSec * TPS)
	if s.Node(f.Node).Speed != 100 {
		t.Fatal("패치가 끝나면 다시 일해야 합니다")
	}
}

// 패치하는 동안에는 장애로 느려진 서버도 멈춘다. 막은 보안 사고는 끝난 장애처럼 정리된다.
func TestPatchBeatsSlowFault(t *testing.T) {
	s, do := hard(t, 1000)
	f := strikeNow(t, s, do, "hack", func(f *Fault) {
		s.addFault(FaultDef{Type: "api_slow", AtSec: s.Tick / TPS, NoticeSec: 0, DurationSec: 30, Percent: 40})
		s.Advance(2)
		slow := s.Faults[len(s.Faults)-1]
		slow.Node = f.Node
		do(Command{Type: "patch", ID: f.Node})
		s.Advance(1)
		if s.Node(f.Node).Speed != 0 {
			t.Fatalf("패치 중인데 속도 %d", s.Node(f.Node).Speed)
		}
	})
	if f.State != "blocked" {
		t.Fatal(f.State)
	}
	for range 30 {
		do(Command{Type: "scene", Kind: "hack"})
		s.Advance(15 * TPS)
	}
	if len(s.Faults) > 26 {
		t.Fatalf("끝난 보안 사고가 쌓임: %d", len(s.Faults))
	}
}

func TestLeakCostsHealth(t *testing.T) {
	s, do := hard(t, 0)
	f := strikeNow(t, s, do, "leak", nil)
	if s.Node(f.Node).Kind != DB || s.Health != s.Cfg.Health.Max-s.Cfg.HardMode.Security.LeakHealth {
		t.Fatalf("유출 뒤 서비스 상태 %d, 대상 %+v", s.Health, s.Node(f.Node))
	}
}

// DDoS의 가짜 요청은 처리 용량을 차지하지만 점수, 수익, 기록, 서비스 상태에는 남지 않는다.
func TestDDoSJunkLeavesNoRecord(t *testing.T) {
	s, do := hard(t, 0)
	f := strikeNow(t, s, do, "ddos", nil)
	if f.State != "active" || s.Node(f.Node).Kind != Source {
		t.Fatalf("DDoS %+v", f)
	}
	s.Advance(f.EndAt - s.Tick + 20*TPS)
	if s.junkID >= 0 {
		t.Fatal("가짜 요청이 생기지 않음")
	}
	if s.Score != 0 || s.Completed != 0 || s.Spawned != 0 || s.Failures != [failKinds]int{} || s.Health != s.Cfg.Health.Max || s.Active != 0 {
		t.Fatalf("점수 %d 완료 %d 요청 %d 실패 %v 상태 %d 진행 중 %d", s.Score, s.Completed, s.Spawned, s.Failures, s.Health, s.Active)
	}
	var done int
	for _, n := range s.Nodes {
		if n.Kind == API {
			done += n.Done
		}
	}
	if done == 0 {
		t.Fatal("가짜 요청도 서버를 거쳐야 합니다")
	}
}

func TestRansomLocksUntilPaid(t *testing.T) {
	s, do := hard(t, 1000)
	f := strikeNow(t, s, do, "ransom", nil)
	s.Advance(1)
	db := s.Node(f.Node)
	if f.State != "active" || db.Kind != DB || db.Speed != 0 {
		t.Fatalf("랜섬웨어 %+v, DB 속도 %d", f, db.Speed)
	}
	before, cost := s.Budget, s.RansomCost()
	if a := do(Command{Type: "ransom", ID: db.ID}); !a.OK {
		t.Fatal(a.Error)
	}
	s.Advance(1)
	if f.State != "ended" || db.Speed != 100 || s.Budget != before-cost {
		t.Fatalf("몸값을 냈는데 %s, 속도 %d, 예산 %d", f.State, db.Speed, s.Budget)
	}
	// 패치했다면 잠기지 않는다.
	f = strikeNow(t, s, do, "ransom", func(f *Fault) { do(Command{Type: "patch", ID: f.Node}) })
	if f.State != "blocked" {
		t.Fatalf("패치했는데 %s", f.State)
	}
}
