package game

import (
	"context"
	"fmt"

	"serverway/internal/sim"
)

const maxSaveTick = 24 * 3600 * sim.TPS

// Save는 판을 다시 계산해 이어 하는 데 필요한 기록이다.
type Save struct {
	Seed   uint64         `json:"seed"`
	Chosen bool           `json:"chosen,omitempty"`
	Rules  int            `json:"rules,omitempty"` // 시작한 규칙 판수. 없으면 1판
	Hard   bool           `json:"hard,omitempty"`  // 하드 모드
	Tick   int            `json:"tick"`
	Score  int            `json:"score"`
	Log    []sim.LogEntry `json:"log"`
}

// Local은 통신 없이 세션 하나를 돌린다. 브라우저 안에서 쓴다.
type Local struct {
	s *Session
	c *Client
}

// OpenLocal은 새 판을 열거나 기록을 다시 계산해 이어 연다. 기록은 시작한 판수의 규칙으로 계산하므로 배포 뒤에도 이어진다.
// 그래도 결과가 다르면(판수를 올리지 않고 규칙을 바꿨다면) 이어 하지 않는다. 다시 시작하면 지금 규칙을 따른다.
func OpenLocal(ctx context.Context, cfg *sim.Config, seed uint64, save *Save) (*Local, error) {
	if save != nil && save.Hard && !cfg.Hard {
		cfg = sim.Hard(cfg)
	}
	s := newSession(cfg, seed)
	if save != nil {
		if save.Tick < 0 || save.Tick > maxSaveTick {
			return nil, fmt.Errorf("저장된 기록이 올바르지 않습니다")
		}
		rules, err := sim.ForRules(cfg, save.Rules)
		if err != nil {
			return nil, err
		}
		s.sim = sim.Replay(rules, save.Seed, save.Log, save.Tick)
		if s.sim.Tick != save.Tick || s.sim.Score != save.Score {
			return nil, fmt.Errorf("저장된 기록을 이어 할 수 없습니다")
		}
		s.sim.Paused, s.sim.Started, s.chosen = true, true, save.Chosen
	}
	l := &Local{s, NewClient()}
	go s.run(ctx, func() {})
	if !post(ctx, s.join, l.c) {
		return nil, ctx.Err()
	}
	return l, nil
}

func (l *Local) Out() <-chan []byte { return l.c.Out }

func (l *Local) Send(ctx context.Context, b []byte) {
	if in, ok := decode(l.c, b); ok {
		post(ctx, l.s.cmds, in)
	}
}
