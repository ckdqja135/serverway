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

// Rules는 지금 규칙의 판수다. 같은 기록이 다른 결과를 내게 바꾸면 하나 올리고, 예전 동작은 판수로 나눠 남겨 둔다.
// 저장된 판은 시작한 판수의 규칙으로 다시 계산하므로, 배포 뒤에도 하던 판을 그대로 이어 간다.
//
//	1: 처음 판수. 저장 기록에 판수가 없으면 1이다
//	2: 1초 손실 상한, 시드로 고정한 운(보상 카드, 장애 종류와 대상, 새 클라이언트 자리 후보)
const Rules = 2

type ServerLevel struct {
	Slots  int `json:"slots"`
	ProcMs int `json:"procMs"`
	Queue  int `json:"queue"`
	Cost   int `json:"cost"`
	Hit    int `json:"hit"` // Redis가 DB 대신 응답하는 비율(퍼센트)
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
	lv.Hit = min(80, lv.Hit+d.Step.Hit*k)
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
	ExpandW           int `json:"expandW"` // 정해진 영역을 다 쓴 뒤 구간마다 넓어지는 칸 수
	ExpandH           int `json:"expandH"`
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
	Step   int    `json:"step,omitempty"` // 구간 열 개마다 늘어나는 양
}

// Area는 그 구간을 넘긴 뒤부터 쓸 수 있는 보드 영역이다.
type Area struct {
	AtPhase int `json:"atPhase"`
	X       int `json:"x"`
	Y       int `json:"y"`
	W       int `json:"w"`
	H       int `json:"h"`
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
	InterestPercent  int    `json:"interestPercent"` // 구간을 넘길 때 가진 예산에 붙는 보너스. 그 구간 수입까지만 준다
	RevenueStep      int    `json:"revenueStep"`     // 구간마다 늘어나는 처리 수익(퍼센트)
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
		MaxLossPerSec int `json:"maxLossPerSec"` // 1초에 잃을 수 있는 최대량. 0이면 상한이 없다
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
	API    ServerDef `json:"api"`
	DB     ServerDef `json:"db"`
	Redis  ServerDef `json:"redis"`
	Unlock struct {  // 열리는 성장 단계
		Storage   int `json:"storage"`
		Redis     int `json:"redis"`
		WebSocket int `json:"websocket"`
	} `json:"unlock"`
	// Storage는 DB의 저장 공간이다. 처리할 때마다 차고, 가득 차면 느려진다.
	Storage struct {
		Base        int `json:"base"`
		GrowPercent int `json:"growPercent"`
		Cost        int `json:"cost"`
		CostStep    int `json:"costStep"`
		WarnPercent int `json:"warnPercent"`
		SlowPercent int `json:"slowPercent"`
	} `json:"storage"`
	WebSocket struct {
		Cost           int `json:"cost"`
		CostStep       int `json:"costStep"`
		SpeedPercent   int `json:"speedPercent"`
		RatePercent    int `json:"ratePercent"`
		RevenuePercent int `json:"revenuePercent"` // 이 링크로 완료한 요청의 처리 수익
	} `json:"websocket"`
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
	Rules    int         `json:"-"` // 따르는 규칙 판수
}

// ForRules는 예전 판수로 시작한 판을 이어 하기 위해 그때의 값으로 되돌린 설정이다. 판수가 없던 기록은 1판이다.
func ForRules(base *Config, rules int) (*Config, error) {
	rules = max(1, rules)
	if rules > Rules {
		return nil, fmt.Errorf("더 새로운 규칙으로 저장된 기록입니다")
	}
	c := *base
	c.Rules = rules
	if rules < 2 {
		c.Health.MaxLossPerSec = 0
	}
	return &c, nil
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
	if c.PhaseSeconds < 1 || len(c.PhaseIncome) == 0 || len(c.Ranks) == 0 || len(c.API.Levels) == 0 || len(c.DB.Levels) == 0 || len(c.Redis.Levels) == 0 || len(c.Link.Levels) == 0 ||
		len(c.Sources) == 0 || c.Link.CellsPerSec < 1 || c.Link.RoomPerCell < 1 || len(c.Areas) == 0 || c.Health.Max < 1 || len(c.Rewards) < 2 {
		return nil, fmt.Errorf("밸런스 설정에 필요한 값이 빠져 있습니다")
	}
	c.Rules = Rules
	return &c, nil
}

func Default() *Config {
	c, err := Parse(nil)
	if err != nil {
		panic(err)
	}
	return c
}
