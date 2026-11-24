package game

import (
	"context"
	"encoding/json"
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
