package game

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"serverway/internal/sim"

	"github.com/coder/websocket"
)

type msg struct {
	Type    string  `json:"type"`
	Epoch   int     `json:"epoch"`
	LastSeq int64   `json:"lastSeq"`
	Head    head    `json:"head"`
	Ack     sim.Ack `json:"ack"`
	Nodes   []struct {
		ID int `json:"id"`
	} `json:"nodes"`
	Links []struct {
		ID int `json:"id"`
	} `json:"links"`
}

type peer struct {
	*testing.T
	c *websocket.Conn
}

func dial(t *testing.T, url, id string) peer {
	t.Helper()
	c, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(url, "http")+"/ws?id="+id, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.SetReadLimit(1 << 20)
	return peer{t, c}
}

func (p peer) until(ok func(msg) bool) msg {
	p.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, b, err := p.c.Read(ctx)
		if err != nil {
			p.Fatal(err)
		}
		var m msg
		if err := json.Unmarshal(b, &m); err != nil {
			p.Fatal(err)
		}
		if ok(m) {
			return m
		}
	}
}

func (p peer) send(s string) {
	p.Helper()
	if err := p.c.Write(context.Background(), websocket.MessageText, []byte(s)); err != nil {
		p.Fatal(err)
	}
}

func kind(k string) func(msg) bool { return func(m msg) bool { return m.Type == k } }

// 접속이 끊기면 멈추고, 다시 연결하면 같은 상태를 받고, 재시작하면 이전 상태가 남지 않는지 확인한다.
func TestReconnectAndRestart(t *testing.T) {
	srv := httptest.NewServer(Handler(NewManager(t.Context(), sim.Default()), t.TempDir()))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/sessions", "application/json", strings.NewReader(`{"seed":5}`))
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&out)

	p := dial(t, srv.URL, out.ID)
	p.until(kind("snapshot"))
	p.send(`{"epoch":1,"seq":1,"type":"link","from":1,"to":2}`)
	p.send(`{"epoch":1,"seq":1,"type":"link","from":1,"to":2}`)
	a := p.until(kind("ack")).Ack
	b := p.until(kind("ack")).Ack
	if !a.OK || a != b {
		t.Fatalf("같은 순번은 같은 결과여야 함: %+v %+v", a, b)
	}
	p.send(`{"epoch":1,"seq":2,"type":"pause","on":false}`)
	p.until(func(m msg) bool { return m.Type == "frame" && m.Head.Tick > 5 })
	p.c.CloseNow()

	time.Sleep(150 * time.Millisecond)
	p = dial(t, srv.URL, out.ID)
	s1 := p.until(kind("snapshot"))
	if !s1.Head.Paused || s1.Head.Why != pauseOffline || s1.LastSeq != 2 || len(s1.Links) != 1 {
		t.Fatalf("재연결 상태가 다름: %+v", s1.Head)
	}
	time.Sleep(200 * time.Millisecond)
	// 다른 창이 이어받으면 이전 창은 명령을 보낼 수 없다.
	q := dial(t, srv.URL, out.ID)
	if s2 := q.until(kind("snapshot")); s2.Head.Tick != s1.Head.Tick {
		t.Fatal("멈춘 동안 시간이 흐름")
	}
	q.send(`{"epoch":1,"seq":3,"type":"restart","seed":9}`)
	s3 := q.until(func(m msg) bool { return m.Type == "snapshot" && m.Epoch == 2 })
	if s3.Head.Tick != 0 || len(s3.Links) != 0 || s3.LastSeq != 0 || s3.Head.Budget != sim.Default().InitialBudget {
		t.Fatalf("이전 상태가 남음: %+v", s3.Head)
	}
	q.send(`{"epoch":1,"seq":4,"type":"link","from":1,"to":2}`) // 이전 판 명령은 무시
	q.send(`{"epoch":2,"seq":1,"type":"speed","speed":3}`)
	if m := q.until(kind("ack")); m.Ack.Seq != 1 || !m.Ack.OK {
		t.Fatalf("%+v", m.Ack)
	}
	if r, _ := http.Get(fmt.Sprintf("%s/ws?id=none", srv.URL)); r.StatusCode != 404 {
		t.Fatal("없는 세션")
	}
}

// 보낼 것이 없는 동안에도 연결이 유지되어야 한다.
func TestIdleConnectionStaysOpen(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	m := NewManager(t.Context(), sim.Default())
	srv := httptest.NewServer(Handler(m, t.TempDir()))
	defer srv.Close()
	s, _ := m.Create(1, false)
	p := dial(t, srv.URL, s.ID)
	p.until(kind("snapshot"))
	go func() {
		time.Sleep(7 * time.Second)
		p.c.Write(context.Background(), websocket.MessageText, []byte(`{"epoch":1,"seq":1,"type":"speed","speed":2}`))
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 9*time.Second)
	defer cancel()
	if _, _, err := p.c.Read(ctx); err != nil {
		t.Fatal("조용한 동안 연결이 끊김: ", err)
	}
}
