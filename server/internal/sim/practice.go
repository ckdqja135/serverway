package sim

import "fmt"

const (
	practiceLow    = 120 // 예산이 이보다 적어지면
	practiceRefill = 240 // 이만큼 다시 채운다
	practiceLoad   = 1000
)

// Practice는 튜토리얼용 설정이다. 비용과 처리 능력은 그대로 두고, 시간에 따라 저절로 일어나는 일을 모두 끈다.
// 클라이언트, 트래픽, 장애, 보상은 scene 명령이 왔을 때만 일어나며 지지 않는다.
func Practice(base *Config) *Config {
	c := *base
	c.Practice = true
	c.PhaseSeconds = 1 << 20
	c.InitialBudget = practiceRefill
	c.Health.LearnSeconds = 1 << 20
	c.Request.StartSec = 1
	c.Areas = []Area{{X: 3, Y: 3, W: 16, H: 8}, {AtPhase: 1, X: 1, Y: 2, W: 20, H: 10}}
	c.Sources = []SourceDef{{X: 5, Y: 9}, {X: 6, Y: 5, AppearSec: 1 << 20}}
	c.StartAPI, c.DB0 = Pos{X: 10, Y: 9}, Pos{X: 16, Y: 7}
	c.Faults = nil
	c.Endless = Endless{}
	return &c
}

// scene은 튜토리얼의 다음 장면을 만든다.
func (s *Sim) scene(c Command) error {
	if !s.Cfg.Practice {
		return fmt.Errorf("튜토리얼에서만 쓸 수 있습니다")
	}
	switch c.Kind {
	case "source":
		for _, n := range s.Nodes {
			if n.Kind == Source && n.Def == 1 {
				return nil
			}
		}
		s.addSource(1)
	case "traffic":
		if c.Index < 0 || c.Index > 10000 {
			return fmt.Errorf("트래픽은 0에서 10000까지입니다")
		}
		s.load = c.Index
	case "fault":
		s.addFault(FaultDef{Type: "api_down", AtSec: s.Tick/TPS + 9, NoticeSec: 8, DurationSec: 8})
	case "reward":
		s.PhaseEnd = s.Tick + 1
	case "hack", "leak", "ddos", "ransom":
		if !s.Cfg.Hard {
			return fmt.Errorf("하드 모드 튜토리얼에서만 쓸 수 있습니다")
		}
		s.addFault(s.securityDef(c.Kind, s.Tick/TPS+10, 9))
	default:
		return fmt.Errorf("알 수 없는 장면입니다")
	}
	return nil
}
