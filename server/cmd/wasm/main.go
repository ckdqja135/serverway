//go:build js && wasm

// wasm: 브라우저 안에서 게임을 돌린다. 워커와 JSON 문자열을 주고받으며, 형식은 WebSocket과 같다.
package main

import (
	"context"
	"encoding/json"
	"syscall/js"

	"serverway/internal/game"
	"serverway/internal/sim"
)

func main() {
	ctx := context.Background()
	post := func(s string) { js.Global().Call("postMessage", s) }
	in := make(chan string, 1024)
	js.Global().Set("serverway", js.FuncOf(func(_ js.Value, a []js.Value) any {
		in <- a[0].String()
		return nil
	}))

	// 첫 메시지는 열 판을 정한다.
	var first struct {
		Seed     uint64     `json:"seed"`
		Practice bool       `json:"practice"`
		Hard     bool       `json:"hard"`
		Save     *game.Save `json:"save"`
	}
	json.Unmarshal([]byte(<-in), &first)
	g, err := game.OpenLocal(ctx, game.Mode(sim.Default(), first.Practice, first.Hard), first.Seed, first.Save)
	if err != nil {
		post(`{"type":"ended"}`)
		return
	}
	go func() {
		for b := range g.Out() {
			post(string(b))
		}
	}()
	for m := range in {
		g.Send(ctx, []byte(m))
	}
}
