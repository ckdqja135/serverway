//go:build !js

package game

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"serverway/internal/sim"

	"github.com/coder/websocket"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func Handler(m *Manager, webDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeJSON(w, 415, map[string]string{"error": "application/json 요청만 받습니다"})
			return
		}
		var in struct {
			Seed     uint64 `json:"seed"`
			Practice bool   `json:"practice"`
			Hard     bool   `json:"hard"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "요청을 읽을 수 없습니다"})
			return
		}
		s, err := m.Create(in.Seed%1000000, in.Practice, in.Hard)
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"id": s.ID})
	})
	mux.HandleFunc("GET /api/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		if m.Get(r.PathValue("id")) == nil {
			writeJSON(w, 404, map[string]string{"error": "끝난 게임입니다"})
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	// 시드와 명령 기록. 같은 결과를 다시 계산할 때 쓴다.
	mux.HandleFunc("GET /api/sessions/{id}/replay", func(w http.ResponseWriter, r *http.Request) {
		s := m.Get(r.PathValue("id"))
		if s == nil {
			writeJSON(w, 404, map[string]string{"error": "끝난 게임입니다"})
			return
		}
		ch := make(chan any, 1)
		if !post(r.Context(), s.calls, func() {
			ch <- map[string]any{"seed": s.sim.Seed, "rules": s.sim.Cfg.Rules, "hard": s.sim.Cfg.Hard, "tick": s.sim.Tick, "score": s.sim.Score, "log": append([]sim.LogEntry{}, s.sim.Log...)}
		}) {
			writeJSON(w, 503, map[string]string{"error": "잠시 후 다시 시도하세요"})
			return
		}
		writeJSON(w, 200, <-ch)
	})
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) { serveWS(m, w, r) })
	files := http.FileServer(http.Dir(webDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(webDir, filepath.Clean("/"+r.URL.Path))); err != nil {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
	return mux
}

func serveWS(m *Manager, w http.ResponseWriter, r *http.Request) {
	s := m.Get(r.URL.Query().Get("id"))
	if s == nil {
		http.Error(w, "끝난 게임입니다", http.StatusNotFound)
		return
	}
	conn, err := websocket.Accept(w, r, nil) // 다른 출처의 접속은 라이브러리가 거절한다.
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4096)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	c := NewClient()
	if !post(ctx, s.join, c) {
		return
	}
	defer post(context.Background(), s.leave, c)
	defer c.Close()

	go func() {
		defer cancel()
		ping := time.NewTicker(3 * time.Second)
		defer ping.Stop()
		for {
			var err error
			// 제한 시간은 실제로 보낼 때부터 잰다. 기다리는 시간까지 재면 조용한 동안 연결이 끊긴다.
			timed := func(fn func(context.Context) error) error {
				wctx, stop := context.WithTimeout(ctx, 3*time.Second)
				defer stop()
				return fn(wctx)
			}
			select {
			case b := <-c.Out:
				err = timed(func(w context.Context) error { return conn.Write(w, websocket.MessageText, b) })
			case <-ping.C:
				err = timed(conn.Ping)
			case <-c.Quit:
				conn.Close(websocket.StatusGoingAway, "다른 창에서 이어서 열렸거나 연결이 느립니다")
				err = context.Canceled
			case <-ctx.Done():
				err = ctx.Err()
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		_, b, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if in, ok := decode(c, b); ok && !post(ctx, s.cmds, in) {
			return
		}
	}
}
