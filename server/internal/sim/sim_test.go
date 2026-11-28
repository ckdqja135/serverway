package sim

import (
	"encoding/json"
	"reflect"
	"testing"
)

type game struct {
	*testing.T
	*Sim
	seq          int64
	src, api, db *Node
}

// 장애와 구간 보상이 없는 단순한 판을 만든다. edit으로 값을 바꾼다.
func setup(t *testing.T, edit func(*Config)) *game {
	t.Helper()
	c := Default()
	c.Faults, c.PhaseSeconds, c.InitialBudget, c.Sources = nil, 100000, 100000, c.Sources[:1]
	c.Sources[0].Rate = [][2]int{{0, 2000}}
	c.Request.StartSec = 0
	c.Health.FailPenalty = 1
	c.Endless, c.RevenueEvery = Endless{}, 0
	if edit != nil {
		edit(c)
	}
	g := &game{T: t, Sim: New(c, 7)}
	g.Paused = false
	g.src, g.api, g.db = g.Nodes[0], g.Nodes[1], g.Nodes[2]
	return g
}

func (g *game) cmd(c Command) Ack {
	g.seq++
	c.Seq = g.seq
	return g.Apply(c)
}

func (g *game) must(c Command) int {
	g.Helper()
	a := g.cmd(c)
	if !a.OK {
		g.Fatalf("명령 실패 %+v: %s", c, a.Error)
	}
	return a.ID
}

func (g *game) wire() (int, int) {
	g.Helper()
	return g.must(Command{Type: "link", From: g.src.ID, To: g.api.ID}), g.must(Command{Type: "link", From: g.api.ID, To: g.db.ID})
}

// run은 매 틱 요청 수가 보존되는지 확인하며 진행한다.
func (g *game) run(sec int) {
	g.Helper()
	for range sec * TPS {
		g.Advance(1)
		g.conserved()
	}
}

func (g *game) conserved() {
	g.Helper()
	seen := map[int]bool{}
	add := func(r *Request) {
		if seen[r.ID] {
			g.Fatalf("요청 %d 복제됨", r.ID)
		}
		seen[r.ID] = true
	}
	for _, n := range g.Nodes {
		for _, r := range n.Queue {
			add(r)
		}
		for _, j := range n.Jobs {
			add(j.Req)
		}
		for _, r := range n.Out {
			add(r)
		}
	}
	for _, l := range g.Links {
		for _, ln := range l.Lanes {
			for _, r := range ln.Moving {
				add(r)
			}
		}
		if l.Pending < 0 {
			g.Fatalf("링크 %d 왕복 수가 음수", l.ID)
		}
	}
	failed := 0
	for _, f := range g.Failures {
		failed += f
	}
	if len(seen) != g.Active || g.Spawned != g.Completed+failed+g.Active {
		g.Fatalf("틱 %d 요청 수 불일치: 발생 %d 완료 %d 실패 %d 진행 %d 실제 %d", g.Tick, g.Spawned, g.Completed, failed, g.Active, len(seen))
	}
}

func fails(s *Sim) int { return s.Failures[0] + s.Failures[1] + s.Failures[2] + s.Failures[3] }

func TestNoLinkNeverCompletes(t *testing.T) {
	g := setup(t, nil)
	g.run(30)
	if g.Completed != 0 || g.Failures[FailNoRoute] == 0 || fails(g.Sim) != g.Failures[FailNoRoute] {
		t.Fatalf("완료 %d 실패 %v", g.Completed, g.Failures)
	}
	// API까지만 연결해도 완료되지 않고, 이어지지 않는 API로는 보내지 않는다.
	g = setup(t, nil)
	g.must(Command{Type: "link", From: g.src.ID, To: g.api.ID})
	g.run(30)
	if g.Completed != 0 || g.Links[0].Passed != 0 {
		t.Fatalf("완료 %d 통과 %d", g.Completed, g.Links[0].Passed)
	}
}

func TestCannotSkipStage(t *testing.T) {
	g := setup(t, nil)
	if a := g.cmd(Command{Type: "link", From: g.src.ID, To: g.db.ID}); a.OK || g.Budget != 100000 {
		t.Fatal("클라이언트에서 DB로 바로 연결되면 안 됨")
	}
	// 규칙을 어긴 링크가 있더라도 단계를 건너뛰지 않는다.
	g.Links = append(g.Links, &Link{ID: 99, From: g.src.ID, To: g.db.ID, Cells: 9, Travel: 5})
	g.reindex()
	g.run(20)
	if g.Completed != 0 {
		t.Fatal("API를 거치지 않고 완료됨")
	}
	r := &Request{ID: 5000, Deadline: 1 << 30}
	g.Active++
	g.Spawned++
	g.arrive(r, g.db)
	if r.State != Failed {
		t.Fatal("단계가 맞지 않는 도착은 실패여야 함")
	}
}

func TestCompletesThroughPath(t *testing.T) {
	g := setup(t, nil)
	g.wire()
	g.run(30)
	if g.Completed < 50 || fails(g.Sim) != 0 || g.Score != g.Completed*g.Cfg.ScorePerComplete {
		t.Fatalf("완료 %d 실패 %v 점수 %d", g.Completed, g.Failures, g.Score)
	}
}

// 완료 시점은 DB 처리가 아니라 응답이 클라이언트로 돌아왔을 때다.
func TestRoundTrip(t *testing.T) {
	g := setup(t, func(c *Config) { c.Sources[0].Rate = [][2]int{{0, 1000}, {1, 1000}, {2, 0}} })
	l1, l2 := g.wire()
	for g.db.Done == 0 {
		g.Advance(1)
	}
	if g.Completed != 0 || g.Link(l2).Pending != 1 || g.Link(l1).Pending == 0 {
		t.Fatal("응답이 돌아오기 전에 완료됨")
	}
	done := g.Tick
	for g.Completed == 0 {
		g.Advance(1)
		g.conserved()
	}
	if back := g.Tick - done; back < g.Link(l1).Travel+g.Link(l2).Travel {
		t.Fatalf("돌아오는 시간이 너무 짧음: %d틱", back)
	}
	g.run(10)
	if g.Active != 0 || g.Link(l1).Pending != 0 || g.Link(l2).Pending != 0 || g.P95() < 1000 {
		t.Fatalf("왕복 뒤 정리되지 않음: 진행 %d, p95 %d", g.Active, g.P95())
	}
}

func TestCacheSkipsDBOnly(t *testing.T) {
	g := setup(t, nil)
	g.Bonus.Cache = 50
	g.wire()
	g.run(60)
	if g.Completed == 0 || fails(g.Sim) != 0 || g.db.Done == 0 || g.db.Done > g.api.Done*7/10 {
		t.Fatalf("완료 %d API %d DB %d", g.Completed, g.api.Done, g.db.Done)
	}
}

func TestQueueLimitAndDeadline(t *testing.T) {
	g := setup(t, func(c *Config) { c.Sources[0].Rate = [][2]int{{0, 8000}}; c.Link.Levels[0].PerSec = 50 })
	g.wire()
	limit, peak, jam, lane := g.ServerLevel(g.api).Queue, 0, false, 0
	for range 20 * TPS {
		g.Advance(1)
		peak, lane = max(peak, len(g.api.Queue)), max(lane, len(g.Links[0].Lanes[Fwd].Moving))
		jam = jam || g.Links[0].Jam
	}
	// 서버 대기열은 상한까지만 차고, 넘친 트래픽은 링크 위에 밀리다가 클라이언트에서 넘친다.
	if peak != limit || !jam || lane != g.LinkRoom(g.Links[0]) || g.src.Fail[FailOverflow] == 0 {
		t.Fatalf("대기열 최대 %d 상한 %d 체증 %v 초과 실패 %d", peak, limit, jam, g.src.Fail[FailOverflow])
	}
	g = setup(t, func(c *Config) {
		c.API.Levels[0].ProcMs = 4000
		c.API.Levels[0].Queue = 50
		c.Link.Levels[0].PerSec = 50
	})
	g.wire()
	g.run(40)
	if g.Failures[FailDeadline] == 0 {
		t.Fatalf("처리 기한 실패가 있어야 함 %v", g.Failures)
	}
}

// 같은 요청량에서 조치별 완료 수를 잰다.
func throughput(t *testing.T, edit func(*Config), act func(*game)) int {
	g := setup(t, edit)
	g.wire()
	if act != nil {
		act(g)
	}
	g.run(60)
	return g.Completed
}

func secondAPI(g *game) {
	id := g.must(Command{Type: "build", X: 10, Y: 10})
	g.must(Command{Type: "link", From: g.src.ID, To: id})
	g.must(Command{Type: "link", From: id, To: g.db.ID})
}

func TestBottlenecksNeedDifferentFixes(t *testing.T) {
	apiBound := func(c *Config) {
		c.Sources[0].Rate = [][2]int{{0, 7000}}
		c.Link.Levels[0].PerSec = 50
		c.DB.Levels[0].Slots, c.DB.Levels[0].Queue = 50, 50
		c.DB.Levels[1] = c.DB.Levels[0]
	}
	base, api := throughput(t, apiBound, nil), throughput(t, apiBound, secondAPI)
	db := throughput(t, apiBound, func(g *game) { g.must(Command{Type: "upgradeNode", ID: g.db.ID}) })
	if api < base*3/2 || db > base*11/10 {
		t.Fatalf("API 병목: 기본 %d, API 증설 %d, DB 개선 %d", base, api, db)
	}
	dbBound := func(c *Config) {
		c.Sources[0].Rate = [][2]int{{0, 7000}}
		c.Link.Levels[0].PerSec = 50
		c.API.Levels[0].Slots = 50
		c.DB.Levels[0].Slots, c.DB.Levels[1].Slots = 1, 4
	}
	base, api = throughput(t, dbBound, nil), throughput(t, dbBound, secondAPI)
	db = throughput(t, dbBound, func(g *game) { g.must(Command{Type: "upgradeNode", ID: g.db.ID}) })
	if db < base*3/2 || api > base*11/10 {
		t.Fatalf("DB 병목: 기본 %d, API 증설 %d, DB 개선 %d", base, api, db)
	}
	linkBound := func(c *Config) {
		c.Sources[0].Rate = [][2]int{{0, 7000}}
		c.API.Levels[0].Slots, c.DB.Levels[0].Slots = 50, 50
	}
	base = throughput(t, linkBound, nil)
	link := throughput(t, linkBound, func(g *game) {
		g.must(Command{Type: "upgradeLink", ID: g.Links[0].ID})
		g.must(Command{Type: "upgradeLink", ID: g.Links[1].ID})
	})
	if link < base*3/2 {
		t.Fatalf("링크 병목: 기본 %d, 링크 개선 %d", base, link)
	}
}

func TestFaultTargetAndDuration(t *testing.T) {
	g := setup(t, func(c *Config) {
		c.Faults = []FaultDef{{Type: "api_down", AtSec: 20, NoticeSec: 5, DurationSec: 6}}
		c.API.Levels[0].Slots = 4
	})
	g.wire()
	secondAPI(g)
	other := g.Nodes[3]
	f := g.Faults[0]
	g.run(14)
	if f.State != "pending" {
		t.Fatal("예고 전이어야 함")
	}
	g.run(2)
	if f.State != "notice" || g.Node(f.Node).Speed != 100 {
		t.Fatal("예고 중에는 영향이 없어야 함")
	}
	target := g.Node(f.Node)
	if target != g.api {
		t.Fatal("가장 많이 쓴 API가 대상이어야 함")
	}
	for range 20 * TPS {
		g.Advance(1)
		in := g.Tick >= 20*TPS && g.Tick < 26*TPS
		if (target.Speed == 0) != in || other.Speed != 100 {
			t.Fatalf("틱 %d 대상 %d 다른 API %d", g.Tick, target.Speed, other.Speed)
		}
	}
	if f.State != "ended" || other.Done == 0 {
		t.Fatal("복구되어야 함")
	}
}

func TestBlockedLinkUsesBypassOnly(t *testing.T) {
	block := func(c *Config) {
		c.Faults = []FaultDef{{Type: "link_block", AtSec: 10, NoticeSec: 3, DurationSec: 30}}
	}
	g := setup(t, block)
	g.wire()
	g.run(39)
	alone := g.Failures[FailFault]
	if alone == 0 {
		t.Fatalf("우회로가 없으면 장애 실패여야 함 %v", g.Failures)
	}
	g = setup(t, block)
	g.wire()
	secondAPI(g)
	g.run(39)
	// 막히는 순간 이미 왕복 중이던 요청만 돌아오지 못한다.
	if fails(g.Sim)*4 > alone || g.Completed < 50 {
		t.Fatalf("우회로가 있는데 실패가 많음 %v, 우회로 없을 때 %d", g.Failures, alone)
	}
}

func TestPauseAndSpeed(t *testing.T) {
	g := setup(t, nil)
	g.wire()
	g.run(5)
	g.must(Command{Type: "pause", On: true})
	tick, spawned := g.Tick, g.Spawned
	if g.Advance(500) != 0 || g.Tick != tick || g.Spawned != spawned {
		t.Fatal("일시 정지 중 시간이 흐름")
	}
	a, b := setup(t, nil), setup(t, nil)
	a.wire()
	b.wire()
	for range 600 {
		a.Advance(1)
	}
	for range 200 {
		b.Advance(3)
	}
	if a.digest() != b.digest() {
		t.Fatal("속도에 따라 결과가 달라짐")
	}
}

func (s *Sim) digest() string {
	type nodeState struct {
		ID, Level, Q, J, Done int
		F                     [failKinds]int
	}
	type linkState struct{ ID, Level, M, Passed, Credit int }
	var v struct {
		Tick, Score, Health, Budget, Spawned, Completed, Active int
		Fail                                                    [failKinds]int
		Rng                                                     uint64
		Over                                                    string
		N                                                       []nodeState
		L                                                       []linkState
	}
	v.Tick, v.Score, v.Health, v.Budget, v.Spawned, v.Completed, v.Active = s.Tick, s.Score, s.Health, s.Budget, s.Spawned, s.Completed, s.Active
	v.Fail, v.Rng, v.Over = s.Failures, s.rng, s.Over
	for _, n := range s.Nodes {
		v.N = append(v.N, nodeState{n.ID, n.Level, len(n.Queue), len(n.Jobs), n.Done, n.Fail})
	}
	for _, l := range s.Links {
		v.L = append(v.L, linkState{l.ID, l.Level, len(l.Lanes[0].Moving)*1000 + len(l.Lanes[1].Moving), l.Passed, l.Lanes[0].credit + l.Lanes[1].credit})
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func TestReplayIsDeterministic(t *testing.T) {
	b := Bot{Link: true, API: true, DB: true, LinkUp: true, Reroute: true}
	s := Play(Default(), 11, &b, 900)
	// 기록을 JSON으로 저장했다 읽어도 같은 결과여야 한다.
	raw, _ := json.Marshal(s.Log)
	var log []LogEntry
	if err := json.Unmarshal(raw, &log); err != nil || !reflect.DeepEqual(log, s.Log) {
		t.Fatal("기록 저장 불일치", err)
	}
	if r := Replay(Default(), 11, log, s.Tick); r.digest() != s.digest() {
		t.Fatalf("재현 불일치\n%s\n%s", r.digest(), s.digest())
	}
}

func TestDuplicateAndInvalidCommands(t *testing.T) {
	g := setup(t, func(c *Config) { c.InitialBudget, c.API.Cost, c.API.CostStep = 60, 55, 0 })
	c := Command{Seq: 1, Type: "build", X: 10, Y: 10}
	a, b := g.Apply(c), g.Apply(c)
	if !a.OK || a != b || g.Budget != 5 || len(g.Nodes) != 4 {
		t.Fatalf("중복 명령이 두 번 반영됨: 예산 %d 노드 %d", g.Budget, len(g.Nodes))
	}
	g.seq = 1
	if a := g.cmd(Command{Type: "build", X: 10, Y: 2}); a.OK || g.Budget != 5 || len(g.Nodes) != 4 {
		t.Fatal("예산이 부족한데 건설됨")
	}
	for _, bad := range []Command{{Type: "link", From: g.db.ID, To: g.api.ID}, {Type: "link", From: 1, To: 999}, {Type: "build", X: 10, Y: 6},
		{Type: "upgradeNode", ID: g.src.ID}, {Type: "removeNode", ID: g.db.ID}, {Type: "reward"}, {Type: "speed", Speed: 9}, {Type: "nope"}} {
		if a := g.cmd(bad); a.OK || a.Error == "" || g.Budget != 5 {
			t.Fatalf("거절되어야 함: %+v", bad)
		}
	}
}

func TestRemovalKeepsRequests(t *testing.T) {
	g := setup(t, func(c *Config) { c.Sources[0].Rate = [][2]int{{0, 3500}} })
	l1, _ := g.wire()
	secondAPI(g)
	g.run(5)
	moving := len(g.Link(l1).Lanes[Fwd].Moving)
	g.must(Command{Type: "removeLink", ID: l1})
	if moving == 0 || len(g.Link(l1).Lanes[Fwd].Moving) != moving {
		t.Fatal("이동 중인 요청이 사라짐")
	}
	passed := g.Link(l1).Passed
	for g.Link(l1) != nil {
		if g.Link(l1).Passed != passed {
			t.Fatal("철거 중인 링크로 새로 보냄")
		}
		if g.Link(l1).Pending > 0 && g.Link(l1).Draining == false {
			t.Fatal("철거 상태가 풀림")
		}
		g.Advance(1)
		g.conserved()
	}
	budget := g.Budget
	g.must(Command{Type: "removeNode", ID: g.api.ID})
	if g.Budget <= budget {
		t.Fatal("환급이 있어야 함")
	}
	for _, l := range g.Links {
		if l.From == g.api.ID && l.Paid != 0 {
			t.Fatal("나가는 링크를 두 번 환급할 수 있음")
		}
	}
	g.run(20)
	if g.Node(g.api.ID) != nil || fails(g.Sim) != 0 {
		t.Fatalf("비운 뒤 철거되어야 함, 실패 %v", g.Failures)
	}
}

func TestLearningFloorAndEnd(t *testing.T) {
	c := Default()
	s := New(c, 1)
	s.Paused = false
	s.Advance(c.Health.LearnSeconds*TPS - 1)
	if s.Over != "" || s.Health < c.Health.LearnFloor {
		t.Fatalf("학습 구간에 패배함: %d", s.Health)
	}
	s = Play(c, 1, &Bot{}, 600)
	if s.Over != "down" || s.Health != 0 || s.Advance(10) != 0 || s.Apply(Command{Seq: 999, Type: "build", X: 10, Y: 10}).OK {
		t.Fatal("끝난 게임이 계속 진행됨")
	}
	if n := New(c, 1); n.Tick != 0 || n.Score != 0 || len(n.Links) != 0 || n.Health != c.Health.Max || n.Pending != nil {
		t.Fatal("새 판에 이전 상태가 남음")
	}
}

// 마라톤은 끝이 정해져 있지 않지만 언젠가는 감당할 수 없게 된다.
// 한쪽만 증설하면 일찍 끝나고, 균형 있게 대응하면 훨씬 오래 버틴다.
func TestMarathonBalance(t *testing.T) {
	for seed := uint64(1); seed <= 5; seed++ {
		worst := 0
		for _, b := range []Bot{{}, {Link: true}, {Link: true, API: true, LinkUp: true, Reroute: true}, {Link: true, DB: true}} {
			s := Play(Default(), seed, &b, 3600)
			worst = max(worst, s.Tick/TPS)
		}
		b := Bot{Link: true, API: true, DB: true, LinkUp: true, Reroute: true, Storage: true}
		s := Play(Default(), seed, &b, 3600)
		if sec := s.Tick / TPS; s.Over != "down" || sec < worst*2 || sec < 480 {
			t.Errorf("시드 %d: 균형 %d초(%s), 한쪽만 증설 최고 %d초", seed, sec, s.Over, worst)
		}
	}
}

// 클라이언트, 보드, 서버, 트래픽이 시간이 지나며 계속 늘어난다.
func TestEndlessGrowth(t *testing.T) {
	c := Default()
	c.Health.FailPenalty, c.Faults = 0, nil
	c.Endless.FaultEverySec = 0
	s := New(c, 3)
	s.Paused = false
	seq := int64(0)
	at := func(sec int) (int, int, int) {
		for s.Tick < sec*TPS {
			s.Advance(TPS)
			s.Take()
			if s.Pending != nil {
				seq++
				s.Apply(Command{Seq: seq, Type: "reward", Index: s.Pending.Next()})
			}
		}
		rate := 0
		for _, n := range servers(s, Source) {
			rate += s.Rate(n, s.Tick)
		}
		a := s.Area()
		return len(servers(s, Source)), a.W * a.H, rate
	}
	pn, pa, pr := at(60)
	for _, sec := range []int{300, 600, 900, 1500} {
		n, area, rate := at(sec)
		if n <= pn || rate <= pr || area < pa || s.Over != "" {
			t.Fatalf("%d초: 클라이언트 %d→%d 면적 %d→%d 트래픽 %d→%d", sec, pn, n, pa, area, pr, rate)
		}
		pn, pa, pr = n, area, rate
	}
	for i, n := range servers(s, Source) {
		for _, o := range servers(s, Source)[i+1:] {
			if dist(n.X, n.Y, o.X, o.Y) < 3 {
				t.Fatal("클라이언트가 겹침")
			}
		}
	}
}

func TestDBShardAndEndlessLevels(t *testing.T) {
	edit := func(c *Config) {
		c.Sources[0].Rate = [][2]int{{0, 5000}}
		c.Link.Levels[0].PerSec, c.API.Levels[0].Slots, c.DB.Levels[0].Slots = 20, 8, 1
	}
	one := throughput(t, edit, nil)
	g := setup(t, edit)
	g.wire()
	cost := g.BuildCost(DB)
	id := g.must(Command{Type: "build", Kind: "db", X: 16, Y: 10})
	if g.BuildCost(DB) <= cost {
		t.Fatal("샤드 비용이 올라야 함")
	}
	g.must(Command{Type: "link", From: g.api.ID, To: id})
	g.run(60)
	if g.Completed < one*3/2 || g.Node(id).Done == 0 {
		t.Fatalf("DB 1대 %d, 샤드 추가 %d", one, g.Completed)
	}
	prev := g.ServerLevel(g.db).Slots
	for range 10 {
		g.must(Command{Type: "upgradeNode", ID: g.db.ID})
		if now := g.ServerLevel(g.db).Slots; now <= prev {
			t.Fatal("레벨을 올려도 커지지 않음")
		} else {
			prev = now
		}
	}
	g.must(Command{Type: "removeNode", ID: id})
	if a := g.cmd(Command{Type: "removeNode", ID: g.db.ID}); a.OK {
		t.Fatal("마지막 DB는 철거할 수 없어야 함")
	}
	g.must(Command{Type: "finish"})
	if g.Over != "done" || g.Advance(5) != 0 {
		t.Fatal("직접 마치기")
	}
}

func TestEventBufferBounded(t *testing.T) {
	g := setup(t, func(c *Config) { c.Sources[0].Rate = [][2]int{{0, 20000}} })
	g.wire()
	g.Advance(300 * TPS)
	if len(g.Events) > maxEvents || g.Active > g.Cfg.Request.MaxActive {
		t.Fatalf("이벤트 %d 진행 %d", len(g.Events), g.Active)
	}
}

// 성장 단계 전에는 쓸 수 없고, 오르면 쓸 수 있다.
func TestUnlocks(t *testing.T) {
	g := setup(t, nil)
	src, _ := g.wire()
	cmds := []Command{{Type: "build", Kind: "redis", X: 13, Y: 4}, {Type: "storage", ID: g.db.ID}, {Type: "websocket", ID: src}}
	for _, c := range cmds {
		if g.cmd(c).OK {
			t.Fatalf("아직 열리지 않아야 함 %+v", c)
		}
	}
	g.Completed = g.Cfg.Ranks[3].At
	for _, c := range cmds {
		g.must(c)
	}
	if g.cmd(cmds[2]).OK {
		t.Fatal("두 번 전환할 수 없어야 함")
	}
	for _, l := range g.Links {
		if l.ID != src && g.cmd(Command{Type: "websocket", ID: l.ID}).OK {
			t.Fatal("클라이언트와 서버 사이에서만 전환해야 함")
		}
	}
}

func TestRedisTakesLoadOffDB(t *testing.T) {
	g := setup(t, func(c *Config) { c.Unlock.Redis = 0 })
	g.wire()
	id := g.must(Command{Type: "build", Kind: "redis", X: 13, Y: 4})
	if g.cmd(Command{Type: "link", From: g.src.ID, To: id}).OK || g.cmd(Command{Type: "link", From: id, To: g.db.ID}).OK {
		t.Fatal("서버에서만 이을 수 있어야 함")
	}
	g.must(Command{Type: "link", From: g.api.ID, To: id})
	g.run(90)
	r := g.Node(id)
	if share := r.Done * 100 / (r.Done + g.db.Done); fails(g.Sim) != 0 || share < 25 || share > 55 {
		t.Fatalf("Redis %d DB %d 실패 %v", r.Done, g.db.Done, g.Failures)
	}
	// 철거하면 남은 요청을 다 돌려보내고, 그 뒤로는 모두 DB로 간다.
	g.must(Command{Type: "removeNode", ID: id})
	g.run(30)
	if g.Node(id) != nil || fails(g.Sim) != 0 {
		t.Fatalf("철거 %v 실패 %v", g.Node(id), g.Failures)
	}
}

func TestStorageFillsAndSlows(t *testing.T) {
	g := setup(t, func(c *Config) { c.Storage.Base, c.Unlock.Storage = 40, 0 })
	g.wire()
	g.run(60)
	limit := g.StorageCap(g.db)
	if g.db.Data != limit || g.Pace(g.db) != g.Cfg.Storage.SlowPercent {
		t.Fatalf("데이터 %d 상한 %d 속도 %d", g.db.Data, limit, g.Pace(g.db))
	}
	g.must(Command{Type: "storage", ID: g.db.ID})
	if g.StorageCap(g.db) <= limit || g.Pace(g.db) != 100 {
		t.Fatal("증설하면 바로 풀려야 함")
	}
	// 샤드를 지으면 공간에 비례해 나뉘고, 철거하면 남은 DB로 옮긴다.
	total := g.db.Data
	id := g.must(Command{Type: "build", Kind: "db", X: 16, Y: 10})
	n := g.Node(id)
	if n.Data == 0 || n.Data >= g.db.Data || n.Data+g.db.Data < total-2 || n.Data+g.db.Data > total {
		t.Fatalf("나눈 뒤 %d + %d, 합계 %d", g.db.Data, n.Data, total)
	}
	before := g.db.Data
	g.must(Command{Type: "removeNode", ID: id})
	g.run(1)
	if g.Node(id) != nil || g.db.Data <= before {
		t.Fatalf("옮긴 뒤 %d, 전 %d", g.db.Data, before)
	}
}

func TestIncomeGrowsWithPhase(t *testing.T) {
	g := setup(t, func(c *Config) { c.PhaseSeconds, c.Endless.IncomeStep = 5, 15 })
	seen := 0
	for phase := 1; phase <= 14; phase++ {
		held := 60 * phase
		g.Budget = held
		for g.Pending == nil {
			g.Advance(1)
			g.Health = g.Cfg.Health.Max
		}
		p := g.Pending
		if p.Income <= seen || p.Interest != min(p.Income, held*g.Cfg.InterestPercent/100) || g.Budget != held+p.Income+p.Interest {
			t.Fatalf("구간 %d 수입 %d(이전 %d) 보너스 %d 예산 %d", phase, p.Income, seen, p.Interest, g.Budget)
		}
		seen = p.Income
		// 미리 보기는 상태를 바꾸지 않는다.
		was := g.digest()
		for _, o := range p.Options {
			g.WithReward(o, func() {})
		}
		if g.digest() != was || g.RevenueMul() != 100+g.Cfg.RevenueStep*phase+g.Bonus.Revenue {
			t.Fatal("미리 보기나 처리 수익이 다름")
		}
		for g.Pending != nil {
			g.must(Command{Type: "reward", Index: g.Pending.Next()})
		}
	}
}

func TestWebSocketIsFasterAndEarnsMore(t *testing.T) {
	play := func(ws bool) (int, int) {
		g := setup(t, func(c *Config) { c.RevenueEvery, c.Unlock.WebSocket = 5, 0 })
		src, _ := g.wire()
		if ws {
			g.must(Command{Type: "websocket", ID: src})
		}
		held := g.Budget
		g.run(60)
		return g.P95(), g.Budget - held
	}
	slow, few := play(false)
	fast, many := play(true)
	if fast >= slow || many <= few {
		t.Fatalf("왕복 %d→%d 수익 %d→%d", slow, fast, few, many)
	}
}

// 빈자리를 서버로 메워도 새 클라이언트는 나온다.
func TestCannotBlockSources(t *testing.T) {
	c := Default()
	c.Health.FailPenalty, c.Faults = 0, nil
	c.Endless.FaultEverySec = 0
	s := New(c, 1)
	s.Paused = false
	seq, born := int64(0), 0
	do := func(cmd Command) bool { seq++; cmd.Seq = seq; return s.Apply(cmd).OK }
	for s.Tick < 900*TPS {
		if s.Pending != nil {
			do(Command{Type: "reward", Index: s.Pending.Next()})
		}
		if s.Tick%(5*TPS) == 0 {
			s.Budget = 1 << 20
			a := s.Area()
			for x := a.X + 1; x < a.X+a.W-1; x++ {
				for y := a.Y + 1; y < a.Y+a.H-1; y++ {
					gap := 99.0
					for _, n := range s.Nodes {
						gap = min(gap, dist(x, y, n.X, n.Y))
					}
					if gap >= 4 && gap <= 6 {
						do(Command{Type: "build", X: x, Y: y})
					}
				}
			}
		}
		s.Advance(1)
		s.Take()
	}
	for _, n := range s.Nodes {
		if n.Kind == Source {
			born++
		}
	}
	if want := len(c.Sources) + s.sourceN - len(s.Coming); born != want {
		t.Fatalf("클라이언트 %d, 나와야 할 수 %d", born, want)
	}
}

// 구간마다 선택지가 2, 3, 4, 5개로 늘고, 넷부터는 두 개를 고른다. 그 뒤로는 계속 다섯 중 둘이다.
func TestRewardChoices(t *testing.T) {
	g := setup(t, func(c *Config) { c.PhaseSeconds, c.Endless.IncomeStep, c.Unlock.Storage = 5, 15, 0 })
	for phase := 1; phase <= 40; phase++ {
		for g.Pending == nil {
			g.Advance(1)
			g.Health = g.Cfg.Health.Max
		}
		if phase == 1 {
			// 보상 창이 뜨기 직전에 시작한 끌기는 이어지고, 짓기는 보상을 고른 뒤에 한다.
			g.must(Command{Type: "link", From: g.src.ID, To: g.api.ID})
			if g.cmd(Command{Type: "build", X: 10, Y: 10}).OK {
				t.Fatal("보상을 고르기 전에는 지을 수 없어야 함")
			}
		}
		p, n, left := g.Pending, min(phase+1, 5), 1
		if n >= 4 {
			left = 2
		}
		ids := map[string]bool{}
		for _, o := range p.Options {
			ids[o.ID] = true
		}
		if len(p.Options) != n || len(ids) != n || p.Left != left {
			t.Fatalf("구간 %d: 선택지 %d(서로 다른 것 %d) 고르는 수 %d", phase, len(p.Options), len(ids), p.Left)
		}
		for i := range left {
			tick := g.Tick
			if g.Advance(5) != 0 || g.Tick != tick {
				t.Fatal("다 고를 때까지 멈춰 있어야 함")
			}
			g.must(Command{Type: "reward", Index: i})
			if i+1 < left && g.cmd(Command{Type: "reward", Index: i}).OK {
				t.Fatal("같은 보상을 두 번 고를 수 없어야 함")
			}
		}
		if g.Pending != nil {
			t.Fatal("다 골랐으면 이어져야 함")
		}
	}
}

// 같은 시드면 어떻게 플레이하든 보상 카드와 장애 종류가 같다.
func TestSeedFixesLuck(t *testing.T) {
	play := func(b Bot) (cards, faults []string) {
		c := Default()
		c.Health.FailPenalty, c.Unlock.Storage = 0, 0 // 빠지는 보상이 플레이에 따라 달라지지 않게 한다
		s := New(c, 42)
		b.do(s, Command{Type: "pause", On: false})
		seen := map[int]bool{}
		for s.Tick < 900*TPS {
			if s.Pending != nil {
				for _, o := range s.Pending.Options {
					cards = append(cards, o.ID)
				}
				cards = append(cards, "|")
			}
			b.Act(s)
			s.Advance(TPS)
			s.Take()
			for _, f := range s.Faults {
				if !seen[f.ID] {
					seen[f.ID] = true
					faults = append(faults, f.Def.Type)
				}
			}
		}
		return
	}
	c1, f1 := play(Bot{Link: true})
	c2, f2 := play(Bot{Link: true, API: true, DB: true, LinkUp: true, Reroute: true, Redis: true})
	if !reflect.DeepEqual(c1, c2) || !reflect.DeepEqual(f1, f2) || len(f1) < 8 {
		t.Fatalf("같은 시드인데 운이 다름\n%v\n%v\n%v\n%v", c1, c2, f1, f2)
	}
}

// 실패가 몰려도 1초에 잃는 서비스 상태에는 상한이 있다.
func TestLossCapPerSecond(t *testing.T) {
	g := setup(t, func(c *Config) {
		c.Health.FailPenalty, c.Health.MaxLossPerSec = 5000, 3000
		c.Sources[0].Rate = [][2]int{{0, 8000}}
	})
	prev, lost := g.Health, 0
	for range 10 {
		g.run(1)
		if lost = prev - g.Health; lost > 3000 {
			t.Fatalf("1초에 %d 잃음", lost)
		}
		prev = g.Health
	}
	if lost != 3000 {
		t.Fatalf("상한까지는 잃어야 함: 마지막 1초 %d", lost)
	}
}
