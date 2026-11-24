// Package sim은 네트워크와 벽시계 없이 동작하는 게임 규칙이다.
// 모든 계산은 정수와 고정 틱으로 하며, 같은 설정, 시드, 명령 기록이면 같은 결과를 낸다.
package sim

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

const (
	TickMs = 50
	TPS    = 1000 / TickMs
)

type ServerLevel struct {
	Slots  int `json:"slots"`
	ProcMs int `json:"procMs"`
	Queue  int `json:"queue"`
	Cost   int `json:"cost"`
}

type LinkLevel struct {
	PerSec int `json:"perSec"`
	Cost   int `json:"cost"`
}

// ServerDef는 서버 종류의 값이다. Levels를 넘는 레벨은 Step만큼 계속 늘어난다.
type ServerDef struct {
	Cost     int           `json:"cost"`
	CostStep int           `json:"costStep"` // 한 대 늘 때마다 오르는 건설 비용
	Step     ServerLevel   `json:"step"`
	Levels   []ServerLevel `json:"levels"`
}

func (d ServerDef) Level(i int) ServerLevel {
	if i < len(d.Levels) {
		return d.Levels[i]
	}
	lv, k := d.Levels[len(d.Levels)-1], i-len(d.Levels)+1
	lv.Slots += d.Step.Slots * k
	lv.Queue += d.Step.Queue * k
	lv.Cost += d.Step.Cost * k
	return lv
}

func (c *Config) LinkLevel(i int) LinkLevel {
	if i < len(c.Link.Levels) {
		return c.Link.Levels[i]
	}
	lv, k := c.Link.Levels[len(c.Link.Levels)-1], i-len(c.Link.Levels)+1
	lv.PerSec += c.Link.Step.PerSec * k
	lv.Cost += c.Link.Step.Cost * k
	return lv
}

// Endless는 정해진 초반 이후 끝없이 이어지는 성장 규칙이다. 간격이 0이면 그 규칙을 끈다.
type Endless struct {
	SourceFromSec     int `json:"sourceFromSec"`
	SourceEverySec    int `json:"sourceEverySec"`
	SourceMinEverySec int `json:"sourceMinEverySec"`
	SourceNoticeSec   int `json:"sourceNoticeSec"`
	RateStart         int `json:"rateStart"`
	RatePerMin        int `json:"ratePerMin"`
	RampMin           int `json:"rampMin"`
	GrowthPerMin      int `json:"growthPerMin"`
	SurgeFromMin      int `json:"surgeFromMin"` // 이때부터 전체 트래픽이 점점 더 빠르게 늘어난다
	SurgePerMin       int `json:"surgePerMin"`
	FaultFromSec      int `json:"faultFromSec"`
	FaultEverySec     int `json:"faultEverySec"`
	FaultMinEverySec  int `json:"faultMinEverySec"`
	ExpandFromSec     int `json:"expandFromSec"`
	ExpandEverySec    int `json:"expandEverySec"`
	MaxW              int `json:"maxW"`
	MaxH              int `json:"maxH"`
	IncomeStep        int `json:"incomeStep"`
	PhaseStepSec      int `json:"phaseStepSec"`
	PhaseMaxSec       int `json:"phaseMaxSec"`
}

type Rank struct {
	At   int    `json:"at"` // 누적 완료 수
	Name string `json:"name"`
}

type SourceDef struct {
	X         int      `json:"x"`
	Y         int      `json:"y"`
	AppearSec int      `json:"appearSec"`
	NoticeSec int      `json:"noticeSec"`
	Rate      [][2]int `json:"rate"` // [초, 초당 요청 수의 1000배]
}

type FaultDef struct {
	Type        string `json:"type"` // api_slow, api_down, link_block
	AtSec       int    `json:"atSec"`
	NoticeSec   int    `json:"noticeSec"`
	DurationSec int    `json:"durationSec"`
	Percent     int    `json:"percent"` // api_slow에서 남는 처리 속도
}

type RewardDef struct {
	ID     string `json:"id"`
	Amount int    `json:"amount"`
}

// Area는 그 시각부터 쓸 수 있는 보드 영역이다.
type Area struct {
	AtSec int `json:"atSec"`
	X     int `json:"x"`
	Y     int `json:"y"`
	W     int `json:"w"`
	H     int `json:"h"`
}

type Pos struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Config struct {
	PhaseSeconds     int    `json:"phaseSeconds"`
	Areas            []Area `json:"areas"`
	InitialBudget    int    `json:"initialBudget"`
	PhaseIncome      []int  `json:"phaseIncome"`
	RefundPercent    int    `json:"refundPercent"`
	MinNodeGap       int    `json:"minNodeGap"`
	RevenueEvery     int    `json:"revenueEvery"` // 이 값에 서버 대수를 더한 만큼 완료할 때마다 예산 1
	MaxServers       int    `json:"maxServers"`
	MaxLinks         int    `json:"maxLinks"`
	ScorePerComplete int    `json:"scorePerComplete"`
	Health           struct {
		Max           int `json:"max"`
		FailPenalty   int `json:"failPenalty"`
		FaultPercent  int `json:"faultPenaltyPercent"` // 장애가 진행 중일 때 페널티 비율
		SuccessGain   int `json:"successGain"`
		MaxGainPerSec int `json:"maxGainPerSec"`
		LearnSeconds  int `json:"learnSeconds"`
		LearnFloor    int `json:"learnFloor"`
	} `json:"health"`
	Request struct {
		DeadlineSec int `json:"deadlineSec"`
		MaxHops     int `json:"maxHops"`
		MaxActive   int `json:"maxActive"`
		StartSec    int `json:"startSec"`
	} `json:"request"`
	Source struct {
		Queue int `json:"queue"`
	} `json:"source"`
	API  ServerDef `json:"api"`
	DB   ServerDef `json:"db"`
	Link struct {
		BaseCost    int         `json:"baseCost"`
		PerCell     int         `json:"perCell"`
		CellsPerSec int         `json:"cellsPerSec"`
		RoomPerCell int         `json:"roomPerCell"` // 한 차선 1칸에 들어가는 요청 수
		Step        LinkLevel   `json:"step"`
		Levels      []LinkLevel `json:"levels"`
	} `json:"link"`
	Endless  Endless     `json:"endless"`
	Ranks    []Rank      `json:"ranks"`
	Sources  []SourceDef `json:"sources"`
	StartAPI Pos         `json:"startApi"`
	DB0      Pos         `json:"db0"`
	Faults   []FaultDef  `json:"faults"`
	Rewards  []RewardDef `json:"rewards"`
	Practice bool        `json:"-"` // 튜토리얼
}

//go:embed balance.json
var defaultBalance []byte

// Parse는 밸런스 설정을 읽는다. data가 비어 있으면 기본값을 쓴다.
func Parse(data []byte) (*Config, error) {
	if len(data) == 0 {
		data = defaultBalance
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.PhaseSeconds < 1 || len(c.PhaseIncome) == 0 || len(c.Ranks) == 0 || len(c.API.Levels) == 0 || len(c.DB.Levels) == 0 || len(c.Link.Levels) == 0 ||
		len(c.Sources) == 0 || c.Link.CellsPerSec < 1 || c.Link.RoomPerCell < 1 || len(c.Areas) == 0 || c.Health.Max < 1 || len(c.Rewards) < 2 {
		return nil, fmt.Errorf("밸런스 설정에 필요한 값이 빠져 있습니다")
	}
	return &c, nil
}

func Default() *Config {
	c, err := Parse(nil)
	if err != nil {
		panic(err)
	}
	return c
}
