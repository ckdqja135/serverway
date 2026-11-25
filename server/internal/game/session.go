package game

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"serverway/internal/sim"
)

const (
	maxSessions  = 50
	idleLimit    = 30 * time.Minute
	outBuffer    = 64
	pauseOffline = "연결이 끊겨 자동으로 멈췄습니다"
)

type Client struct {
	Out  chan []byte
	Quit chan struct{}
	once sync.Once
}

func NewClient() *Client {
	return &Client{Out: make(chan []byte, outBuffer), Quit: make(chan struct{})}
}

func (c *Client) Close() { c.once.Do(func() { close(c.Quit) }) }

// Session은 게임 하나다. 모든 상태는 run 고루틴 하나에서만 바꾼다.
type Session struct {
	ID     string
	cfg    *sim.Config
	sim    *sim.Sim
	epoch  int
	why    string
	chosen bool // 시드를 직접 고른 판. 외운 판일 수 있어 최고 기록에서 뺀다
	client *Client
	delta  *delta
	dirty  bool
	seen   time.Time

	join  chan *Client
	leave chan *Client
	cmds  chan inbound
	calls chan func()
}

type inbound struct {
	from  *Client
	epoch int
	cmd   sim.Command
	seed  uint64
}

// decode는 화면이 보낸 명령 하나를 읽는다.
func decode(c *Client, b []byte) (inbound, bool) {
	var in struct {
		Epoch int    `json:"epoch"`
		Seed  uint64 `json:"seed"`
		sim.Command
	}
	if json.Unmarshal(b, &in) != nil {
		return inbound{}, false
	}
	return inbound{c, in.Epoch, in.Command, in.Seed}, true
}

func randomSeed() uint64 {
	var b [8]byte
	rand.Read(b[:])
	return uint64(binary.LittleEndian.Uint32(b[:]))%999999 + 1
}

func newSession(cfg *sim.Config, seed uint64) *Session {
	var b [16]byte
	rand.Read(b[:])
	chosen := seed != 0
	if !chosen {
		seed = randomSeed()
	}
	return &Session{ID: hex.EncodeToString(b[:]), cfg: cfg, sim: sim.New(cfg, seed), epoch: 1, chosen: chosen, delta: newDelta(), seen: time.Now(),
		join: make(chan *Client), leave: make(chan *Client), cmds: make(chan inbound, 32), calls: make(chan func())}
}

// run은 벽시계 50ms마다 속도만큼 틱을 진행하고 100ms마다 변경분을 보낸다.
func (s *Session) run(ctx context.Context, done func()) {
	defer done()
	t := time.NewTicker(sim.TickMs * time.Millisecond)
	defer t.Stop()
	for n := 0; ; n++ {
		select {
		case <-ctx.Done():
			s.drop()
			return
		case c := <-s.join:
			if s.client != nil {
				s.client.Close()
			}
			s.client, s.seen = c, time.Now()
			s.snapshot()
		case c := <-s.leave:
			if s.client == c {
				s.client = nil
				s.seen = time.Now()
				// 접속 문제로 지지 않도록 멈춘다. 틱 사이의 일이라 결과에는 영향이 없다.
				if !s.sim.Paused && s.sim.Over == "" {
					s.sim.Paused = true
					s.why = pauseOffline
				}
			}
		case in := <-s.cmds:
			s.handle(in)
		case fn := <-s.calls:
			fn()
		case <-t.C:
			if s.client == nil {
				if time.Since(s.seen) > idleLimit {
					return
				}
				continue
			}
			if s.sim.Advance(s.sim.Speed) > 0 {
				s.dirty = true
			}
			if n%2 == 0 && s.dirty {
				s.frame()
			}
		}
	}
}

func (s *Session) drop() {
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
}

func (s *Session) send(v any) {
	if s.client == nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case s.client.Out <- b:
	default:
		// 화면이 따라오지 못하면 끊는다. 다시 연결하면 전체 상태를 새로 받는다.
		s.client.Close()
	}
}

func (s *Session) handle(in inbound) {
	if in.from != s.client {
		return
	}
	s.seen = time.Now()
	if in.epoch != s.epoch {
		return // 재시작 이전 판에 보낸 명령
	}
	if in.cmd.Type == "restart" {
		seed := in.seed
		if s.chosen = seed != 0; !s.chosen {
			seed = randomSeed()
		}
		s.epoch++
		s.sim, s.why = sim.New(s.cfg, seed), ""
		s.snapshot()
		return
	}
	if in.cmd.Type == "save" {
		s.send(map[string]any{"type": "save", "epoch": s.epoch, "save": Save{s.sim.Seed, s.chosen, s.sim.Tick, s.sim.Score, s.sim.Log}})
		return
	}
	a := s.sim.Apply(in.cmd)
	if in.cmd.Type == "pause" && a.OK {
		s.why = ""
	}
	s.send(map[string]any{"type": "ack", "epoch": s.epoch, "ack": a})
	s.frame()
}

type movingView [5]int

func (s *Session) snapshot() {
	if s.client == nil {
		return
	}
	s.delta = newDelta()
	nodes, links, _, _ := s.delta.diff(s.sim)
	moving := []movingView{}
	for _, l := range s.sim.Links {
		for _, ln := range l.Lanes {
			for _, r := range ln.Moving {
				moving = append(moving, movingView{r.ID, l.ID, r.Depart, r.Arrive, r.Stage})
			}
		}
	}
	s.sim.Take()
	s.dirty = false
	c := s.cfg
	s.send(map[string]any{"type": "snapshot", "epoch": s.epoch, "seed": s.sim.Seed, "chosen": s.chosen, "lastSeq": s.sim.LastSeq(),
		"config": map[string]any{"practice": c.Practice, "tps": sim.TPS, "healthMax": c.Health.Max,
			"linkBase": c.Link.BaseCost, "linkPerCell": c.Link.PerCell, "minNodeGap": c.MinNodeGap, "maxServers": c.MaxServers,
			"learnSeconds": c.Health.LearnSeconds, "startSec": c.Request.StartSec, "sources": reserved(c), "refundPercent": c.RefundPercent,
			"score": c.ScorePerComplete, "failPenalty": c.Health.FailPenalty, "faultPercent": c.Health.FaultPercent,
			"redisHit": c.Redis.Levels[0].Hit, "wsSpeed": c.WebSocket.SpeedPercent, "wsRate": c.WebSocket.RatePercent, "wsRevenue": c.WebSocket.RevenuePercent,
			"storageWarn": c.Storage.WarnPercent, "storageSlow": c.Storage.SlowPercent},
		"head": makeHead(s.sim, s.why), "nodes": nodes, "links": links, "moving": moving, "result": makeResult(s.sim)})
}

func reserved(c *sim.Config) []sim.Pos {
	var out []sim.Pos
	for _, d := range c.Sources {
		out = append(out, sim.Pos{X: d.X, Y: d.Y})
	}
	return out
}

func (s *Session) frame() {
	nodes, links, gn, gl := s.delta.diff(s.sim)
	s.dirty = false
	s.send(map[string]any{"type": "frame", "epoch": s.epoch, "head": makeHead(s.sim, s.why), "nodes": nodes, "links": links,
		"goneNodes": gn, "goneLinks": gl, "events": s.sim.Take(), "result": makeResult(s.sim)})
}

type Manager struct {
	mu  sync.Mutex
	all map[string]*Session
	cfg *sim.Config
	ctx context.Context
}

func NewManager(ctx context.Context, cfg *sim.Config) *Manager {
	return &Manager{all: map[string]*Session{}, cfg: cfg, ctx: ctx}
}

func (m *Manager) Create(seed uint64, practice bool) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.all) >= maxSessions {
		return nil, fmt.Errorf("동시에 열 수 있는 게임 수를 넘었습니다")
	}
	cfg := m.cfg
	if practice {
		cfg = sim.Practice(cfg)
	}
	s := newSession(cfg, seed)
	m.all[s.ID] = s
	go s.run(m.ctx, func() {
		m.mu.Lock()
		delete(m.all, s.ID)
		m.mu.Unlock()
	})
	return s, nil
}

func (m *Manager) Get(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.all[id]
}

// post는 이미 끝난 세션에 보내다 멈추지 않게 한다.
func post[T any](ctx context.Context, ch chan T, v T) bool {
	select {
	case ch <- v:
		return true
	case <-ctx.Done():
		return false
	case <-time.After(2 * time.Second):
		return false
	}
}
