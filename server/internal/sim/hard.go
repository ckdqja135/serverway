package sim

import (
	"fmt"
	"slices"
)

// Hard는 하드 모드 설정이다. 스타트업 규모로 이미 갖춘 서비스에서 보호 시간 없이 시작하고, 장애에 더해 보안 사고가 일어난다.
// 성장 단계는 스타트업부터 세며, 기능이 열리는 단계도 그만큼 당긴다.
func Hard(base *Config) *Config {
	c := *base
	h := base.HardMode
	c.Hard = true
	c.InitialBudget = h.Budget
	c.Health.LearnSeconds = 0
	c.Areas, c.Sources = h.Areas, h.Sources
	c.StartAPI, c.DB0 = h.APIs[0], h.DBs[0]
	c.Faults = nil
	c.Endless.FaultFromSec, c.Endless.SourceFromSec = h.FaultFromSec, h.SourceFromSec
	c.Ranks = slices.Clone(base.Ranks[1:])
	c.Ranks[0].At = 0
	c.Unlock.Storage, c.Unlock.Redis, c.Unlock.WebSocket = max(0, base.Unlock.Storage-1), max(0, base.Unlock.Redis-1), max(0, base.Unlock.WebSocket-1)
	return &c
}

// HardPractice는 하드 모드 튜토리얼이다. 하드 모드와 같은 판에서 시간에 따라 저절로 일어나는 일을 끄고,
// 보안 사고는 scene 명령이 왔을 때만 일으킨다.
func HardPractice(base *Config) *Config {
	c := Hard(base)
	c.Practice = true
	c.PhaseSeconds = 1 << 20
	c.Health.LearnSeconds = 1 << 20
	c.Request.StartSec = 1
	c.Endless = Endless{}
	c.HardMode.Security.EverySec = 0
	c.Sources = slices.DeleteFunc(slices.Clone(c.Sources), func(d SourceDef) bool { return d.AppearSec > 0 })
	return c
}

// layout은 하드 모드의 처음 서버, DB, 링크를 놓는다. 처음부터 있던 것이라 철거해도 돌려받지 않는다.
func (s *Sim) layout() {
	h := s.Cfg.HardMode
	for _, p := range h.APIs {
		s.addNode(API, p.X, p.Y)
	}
	for _, p := range h.DBs {
		s.addNode(DB, p.X, p.Y)
	}
	start := slices.Clone(s.Nodes)
	for _, l := range h.Links {
		s.link(start[l[0]], start[l[1]], 0)
	}
	s.secAt = h.Security.FromSec * TPS
}

// security는 하드 모드의 보안 사고인지 답한다. 보안 사고는 장애 중 실패 페널티 감면을 받지 않는다.
func security(t string) bool { return t == "hack" || t == "leak" || t == "ddos" || t == "ransom" }

var securityTypes = []string{"hack", "ddos", "ransom", "leak"}

// securityDef는 at초에 공격하는 보안 사고다. Percent는 화면에 보여 줄 피해 크기다.
func (s *Sim) securityDef(t string, at, notice int) FaultDef {
	c := s.Cfg.HardMode.Security
	d := FaultDef{Type: t, AtSec: at, NoticeSec: notice}
	switch t {
	case "hack":
		d.Percent = c.HackPercent
	case "leak":
		d.Percent = c.LeakHealth * 100 / s.Cfg.Health.Max
	case "ddos":
		d.DurationSec = c.DDoSSec
	case "ransom":
		d.DurationSec = c.RansomSec
	}
	return d
}

// stepSecurity는 다음 보안 사고를 장애처럼 미리 예약한다. 종류는 시드로 정한다.
func (s *Sim) stepSecurity() {
	c := s.Cfg.HardMode.Security
	if !s.Cfg.Hard || c.EverySec == 0 || s.Tick != s.secAt-30*TPS {
		return
	}
	t := securityTypes[s.luck(luckSecurity, s.secN, 0, len(securityTypes))]
	s.addFault(s.securityDef(t, s.secAt/TPS, c.NoticeSec))
	s.secN++
	s.secAt += max(c.MinEverySec, c.EverySec-s.secN*3) * TPS
}

// strike는 예고한 보안 사고가 공격하는 순간이다. 패치했으면 막고, 아니면 피해를 준다.
// 해킹과 유출은 그 자리에서 끝나고, DDoS와 랜섬웨어는 정해진 시간 동안 이어진다.
func (s *Sim) strike(f *Fault) {
	st := s.stat(f.Def.Type)
	st.Count++
	if f.Patched {
		f.State = "blocked"
		st.Blocked++
		s.emit(Event{K: "sec", Node: f.Node, A: f.ID, S: 1, Text: f.Def.Type})
		return
	}
	switch f.Def.Type {
	case "hack":
		f.Value = s.Budget * f.Def.Percent / 100
		s.Budget -= f.Value
		f.State = "ended"
	case "leak":
		f.Value = min(s.Health, s.Cfg.HardMode.Security.LeakHealth)
		s.Health -= f.Value
		if s.Tick < s.Cfg.Health.LearnSeconds*TPS && s.Health < s.Cfg.Health.LearnFloor {
			s.Health = s.Cfg.Health.LearnFloor
		}
		f.State = "ended"
	default:
		f.State = "active"
	}
	st.Lost += f.Value
	s.emit(Event{K: "sec", Node: f.Node, A: f.ID, B: f.Value, Text: f.Def.Type})
}

// Threat는 그 노드를 노리는, 아직 패치하지 않은 보안 사고 예고다.
func (s *Sim) Threat(id int) *Fault {
	for _, f := range s.Faults {
		if f.Node == id && f.State == "notice" && security(f.Def.Type) && !f.Patched {
			return f
		}
	}
	return nil
}

// Locked는 그 DB를 잠근 랜섬웨어다.
func (s *Sim) Locked(id int) *Fault {
	for _, f := range s.Faults {
		if f.Node == id && f.State == "active" && f.Def.Type == "ransom" {
			return f
		}
	}
	return nil
}

func (s *Sim) PatchCost() int  { c := s.Cfg.HardMode.Security; return c.PatchCost + c.PatchStep*s.Phase }
func (s *Sim) RansomCost() int { c := s.Cfg.HardMode.Security; return c.RansomCost + c.RansomStep*s.Phase }

// patch는 공격 예고를 받은 곳에 보안 패치를 한다. 패치하는 동안 서버와 DB는 멈추고 클라이언트는 요청을 보내지 않는다.
func (s *Sim) patch(id int) error {
	f := s.Threat(id)
	if f == nil {
		return fmt.Errorf("보안 패치가 필요한 곳이 아닙니다")
	}
	if err := s.pay(s.PatchCost()); err != nil {
		return err
	}
	f.Patched = true
	s.Node(id).PatchUntil = s.Tick + s.Cfg.HardMode.Security.PatchSec*TPS
	return nil
}

// ransom은 몸값을 내고 잠긴 DB를 바로 푼다.
func (s *Sim) ransom(id int) error {
	f := s.Locked(id)
	if f == nil {
		return fmt.Errorf("잠긴 DB가 아닙니다")
	}
	cost := s.RansomCost()
	if err := s.pay(cost); err != nil {
		return err
	}
	f.State, f.Value = "ended", cost
	s.stat("ransom").Lost += cost
	s.emit(Event{K: "fault", A: f.ID, Text: "paid"})
	return nil
}

// spawnJunk는 DDoS의 가짜 요청을 만든다. 번호가 음수이며, 처리 용량과 차선은 똑같이 차지하지만
// 완료해도 점수와 수익이 없고 실패해도 서비스 상태가 줄지 않는다.
func (s *Sim) spawnJunk(n *Node) {
	s.junkID--
	r := &Request{ID: s.junkID, State: Waiting, Source: n.ID, Born: s.Tick, Deadline: s.Tick + s.Cfg.Request.DeadlineSec*TPS}
	s.Active++
	if len(n.Queue) >= s.Cfg.Source.Queue || s.Active > s.Cfg.Request.MaxActive {
		s.fail(r, FailOverflow, n, nil)
		return
	}
	n.Queue = append(n.Queue, r)
}

func (r *Request) junk() bool { return r.ID < 0 }
