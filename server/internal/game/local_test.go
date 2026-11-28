package game

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"serverway/internal/sim"
)

// 저장한 기록으로 이어 열면 같은 자리에서 멈춘 채 시작하고, 맞지 않는 기록은 열지 않는다.
func TestLocalResume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	type reply struct {
		Type  string  `json:"type"`
		Epoch int     `json:"epoch"`
		Ack   sim.Ack `json:"ack"`
		Head  head    `json:"head"`
		Save  Save    `json:"save"`
	}
	wait := func(l *Local, ok func(reply) bool) reply {
		t.Helper()
		for {
			select {
			case b := <-l.Out():
				var r reply
				if err := json.Unmarshal(b, &r); err != nil {
					t.Fatal(err)
				}
				if ok(r) {
					return r
				}
			case <-ctx.Done():
				t.Fatal("시간 초과")
			}
		}
	}
	say := func(l *Local, v string) { l.Send(ctx, []byte(v)) }
	l, err := OpenLocal(ctx, sim.Default(), 7, nil)
	if err != nil {
		t.Fatal(err)
	}
	wait(l, func(r reply) bool { return r.Type == "snapshot" })
	say(l, `{"epoch":1,"seq":1,"type":"link","from":1,"to":2}`)
	say(l, `{"epoch":1,"seq":2,"type":"link","from":2,"to":3}`)
	say(l, `{"epoch":1,"seq":3,"type":"speed","speed":3}`)
	say(l, `{"epoch":1,"seq":4,"type":"pause"}`)
	wait(l, func(r reply) bool { return r.Head.Completed >= 2 })
	say(l, `{"epoch":1,"type":"save"}`)
	save := wait(l, func(r reply) bool { return r.Type == "save" }).Save

	again, err := OpenLocal(ctx, sim.Default(), 0, &save)
	if err != nil {
		t.Fatal(err)
	}
	h := wait(again, func(r reply) bool { return r.Type == "snapshot" }).Head
	if h.Tick != save.Tick || h.Score != save.Score || !h.Paused || !h.Started {
		t.Fatalf("이어 연 상태 %+v, 저장 %d틱 %d점", h, save.Tick, save.Score)
	}
	save.Score++
	if _, err := OpenLocal(ctx, sim.Default(), 0, &save); err == nil {
		t.Fatal("맞지 않는 기록이 열렸습니다")
	}
}

// 배포로 규칙이 바뀌어도 하던 판은 시작한 판수의 규칙으로 다시 계산해 그대로 이어진다.
// testdata/rules1.json은 판수가 생기기 전 코드가 저장한 기록이다.
// 이 테스트가 깨지면 같은 기록의 결과를 바꾼 것이다. sim.Rules를 올리고 예전 동작을 판수로 나눠 남긴 뒤
// RULES_SAVE=write go test ./internal/game -run TestOldRulesResume 로 새 판수의 기록을 만든다.
func TestOldRulesResume(t *testing.T) {
	type fixture struct {
		Save      Save `json:"save"`
		Health    int  `json:"health"`
		Budget    int  `json:"budget"`
		Completed int  `json:"completed"`
	}
	path := func(v int) string { return fmt.Sprintf("testdata/rules%d.json", v) }
	if os.Getenv("RULES_SAVE") == "write" {
		b := sim.Bot{Link: true, API: true, DB: true, LinkUp: true, Reroute: true, Storage: true, Redis: true, WS: true}
		s := sim.New(sim.Default(), 5)
		s.Paused = false
		for s.Over == "" && s.Tick < 700*sim.TPS {
			b.Act(s)
			s.Advance(sim.TPS)
			s.Take()
		}
		out, _ := json.MarshalIndent(fixture{Save{Seed: s.Seed, Rules: sim.Rules, Tick: s.Tick, Score: s.Score, Log: s.Log}, s.Health, s.Budget, s.Completed}, "", " ")
		if err := os.WriteFile(path(sim.Rules), out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for v := 1; v <= sim.Rules; v++ {
		var f fixture
		b, err := os.ReadFile(path(v))
		if err == nil {
			err = json.Unmarshal(b, &f)
		}
		if err != nil {
			t.Fatalf("%d판 기록: %v", v, err)
		}
		cfg, err := sim.ForRules(sim.Default(), f.Save.Rules)
		if err != nil {
			t.Fatal(err)
		}
		s := sim.Replay(cfg, f.Save.Seed, f.Save.Log, f.Save.Tick)
		if s.Tick != f.Save.Tick || s.Score != f.Save.Score || s.Health != f.Health || s.Budget != f.Budget || s.Completed != f.Completed {
			t.Errorf("%d판 기록이 다르게 계산됨: %d틱 %d점 상태 %d 예산 %d 완료 %d, 기록 %d틱 %d점 상태 %d 예산 %d 완료 %d",
				v, s.Tick, s.Score, s.Health, s.Budget, s.Completed, f.Save.Tick, f.Save.Score, f.Health, f.Budget, f.Completed)
		}
		if _, err := OpenLocal(ctx, sim.Default(), 0, &f.Save); err != nil {
			t.Errorf("%d판 기록을 이어 열지 못함: %v", v, err)
		}
	}
}
