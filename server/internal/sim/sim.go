package sim

import (
	"math"
	"slices"
)

type Kind int

const (
	Source Kind = iota
	API
	DB
	Cache // Redis
)

// 실패 사유
const (
	FailNoRoute = iota
	FailOverflow
	FailDeadline
	FailFault
	failKinds
)

type State string

const (
	Waiting    State = "waiting"    // 클라이언트나 서버의 대기열
	Moving     State = "moving"     // 링크 위
	Processing State = "processing" // 서버 슬롯
	Sending    State = "sending"    // 처리를 마치고 다음 링크를 기다림
	Completed  State = "completed"
	Failed     State = "failed"
)

// 처리 단계. 요청은 API와 DB를 거친 뒤 지나온 링크의 반대 차선으로 되돌아온다.
const (
	ToAPI      = iota // API 처리가 필요함
	ToDB              // DB 처리가 필요함
	BackToAPI         // 응답이 API로 돌아가는 중
	BackToUser        // 응답이 클라이언트로 돌아가는 중
	Finished
)

type Request struct {
	ID       int
	Stage    int
	State    State
	Source   int
	Born     int
	Deadline int
	Hops     int
	Depart   int
	Arrive   int
	Path     []int // 지나왔고 아직 되돌아가지 않은 링크
	Hit      bool  // DB 대신 Redis가 응답한다
}

// want는 다음 처리를 할 노드 종류다.
func (r *Request) want() Kind {
	if k := stageKind(r.Stage); k == API || !r.Hit {
		return k
	}
	return Cache
}

type Job struct {
	Req   *Request
	Start int
	Left  int // 남은 작업량. 정상 속도는 틱마다 100
	Done  bool
}

type Node struct {
	ID       int
	Kind     Kind
	Index    int // 같은 종류 안의 번호
	X, Y     int
	Level    int
	Paid     int
	Queue    []*Request
	Jobs     []*Job
	Out      []*Request // API를 거쳐 돌아가는 응답 중 차선을 기다리는 것
	Pending  int        // 이 노드를 거쳐 왕복 중인 요청 수
	Draining bool
	Speed    int // 처리 속도 퍼센트. 장애가 바꾼다
	Def      int // 클라이언트 설정 번호. -1이면 마라톤에서 새로 생긴 클라이언트
	Born     int
	acc      int
	next     int
	Spawned  int
	Done     int
	Fail     [failKinds]int
	Stuck    int // 처리를 마쳤지만 보내지 못하는 요청 수
	Data     int // DB에 쌓인 데이터
	Store    int // 저장 공간 증설 단계
}

// Lane은 한 방향 차선이다. 앞이 막히면 뒤 요청이 줄을 선다.
type Lane struct {
	Moving []*Request
	credit int
	ring   [TPS]int
	Used   int // 최근 1초 동안 들어온 요청 수
}

const (
	Fwd  = 0
	Back = 1
)

type Link struct {
	ID       int
	From, To int
	Level    int
	Paid     int
	Cells    int
	Travel   int
	Lanes    [2]Lane
	Pending  int // 이 링크로 나갔다가 아직 돌아오지 않은 요청 수
	Draining bool
	Blocked  bool
	WS       bool // WebSocket으로 전환한 링크
	Jam      bool // 도착 서버가 꽉 차서 맨 앞 요청이 들어가지 못함
	Passed   int
	Waiting  int // 이 링크로 나가려고 기다리는 요청 수
	Fail     [failKinds]int
}

type Fault struct {
	ID       int
	Def      FaultDef
	State    string // pending, notice, active, ended, canceled
	Node     int
	Link     int
	StartAt  int
	EndAt    int
	FailedIn int // 장애 중 실패 수
}

// FaultStat은 장애 종류별 누적 기록이다. 끝난 장애 자체는 최근 것만 남긴다.
type FaultStat struct {
	Type   string `json:"type"`
	Count  int    `json:"count"`
	Failed int    `json:"failed"`
}

// Coming은 나타날 예정인 클라이언트다.
type Coming struct {
	X, Y, At int
	Def      int
}

type Reward struct {
	Phase    int
	Income   int
	Held     int // 구간을 넘길 때 가지고 있던 예산
	Interest int
	Options  []RewardDef
	Taken    []bool // 이미 고른 것
	Left     int    // 더 고를 수 있는 수
}

// Next는 아직 고르지 않은 첫 보상이다.
func (r *Reward) Next() int { return slices.Index(r.Taken, false) }

// Event는 화면 표시용이다. 게임 계산에는 쓰지 않는다.
type Event struct {
	K    string `json:"k"`
	At   int    `json:"at"`
	Req  int    `json:"r,omitempty"`
	Node int    `json:"n,omitempty"`
	Link int    `json:"l,omitempty"`
	A    int    `json:"a,omitempty"`
	B    int    `json:"b,omitempty"`
	S    int    `json:"s,omitempty"`
	Text string `json:"t,omitempty"`
}

const maxEvents = 4096

type Sim struct {
	Cfg       *Config
	Seed      uint64
	Tick      int
	Paused    bool
	Speed     int
	Started   bool
	Over      string // "", down(서비스 중단), done(직접 마침)
	Score     int
	Health    int
	Budget    int
	Nodes     []*Node
	Links     []*Link
	Faults    []*Fault
	Stats     []FaultStat
	Coming    []Coming
	Pending   *Reward
	Phase     int
	PhaseAt   int // 이번 구간이 시작한 틱
	PhaseEnd  int
	PeakRPS   int
	Spawned   int
	Completed int
	Failures  [failKinds]int
	Active    int
	PeakWait  int
	Bonus     struct{ Queue, Deadline, LinkRate, APIOpt, DBIndex, Cache, Storage, Revenue int }
	Events    []Event
	Log       []LogEntry
	Spent     int

	rng      uint64
	nextID   int
	nextReq  int
	gainLeft int
	lossLeft int // 이번 1초 동안 더 잃을 수 있는 서비스 상태
	revenue  int
	nodeIx   map[int]*Node
	linkIx   map[int]*Link
	outs     map[int][]*Link
	ins      map[int][]*Link
	faultID  int
	faultAt  int // 다음 마라톤 장애의 시작 틱
	faultN   int
	sourceAt int // 다음 마라톤 클라이언트가 나타나는 틱
	sourceN  int
	load     int // 튜토리얼: 클라이언트마다 초당 요청 수의 1000배
	lastSeq  int64
	lats     [100]int // 최근 완료 요청의 왕복 틱 수
	latN     int
	perSec   [10]int // 최근 10초의 초별 완료 수
	acks     map[int64]Ack
}

func New(cfg *Config, seed uint64) *Sim {
	s := &Sim{Cfg: cfg, Seed: seed, rng: seed, Speed: 1, Paused: true, Health: cfg.Health.Max, Budget: cfg.InitialBudget, load: practiceLoad, acks: map[int64]Ack{}}
	for i, d := range cfg.Sources {
		if d.AppearSec == 0 {
			s.addSource(i)
		}
	}
	s.addNode(API, cfg.StartAPI.X, cfg.StartAPI.Y)
	s.addNode(DB, cfg.DB0.X, cfg.DB0.Y)
	for _, f := range cfg.Faults {
		s.addFault(f)
	}
	s.faultAt, s.sourceAt = cfg.Endless.FaultFromSec*TPS, cfg.Endless.SourceFromSec*TPS
	s.PhaseEnd = cfg.PhaseSeconds * TPS
	return s
}

func (s *Sim) addFault(f FaultDef) {
	s.faultID++
	s.Faults = append(s.Faults, &Fault{ID: s.faultID, Def: f, State: "pending", StartAt: f.AtSec * TPS, EndAt: (f.AtSec + f.DurationSec) * TPS})
}

// reindex는 노드나 링크가 바뀔 때 조회용 색인을 다시 만든다. 순서는 만든 순서 그대로다.
func (s *Sim) reindex() {
	s.nodeIx, s.linkIx = make(map[int]*Node, len(s.Nodes)), make(map[int]*Link, len(s.Links))
	s.outs, s.ins = map[int][]*Link{}, map[int][]*Link{}
	for _, n := range s.Nodes {
		s.nodeIx[n.ID] = n
	}
	for _, l := range s.Links {
		s.linkIx[l.ID] = l
		s.outs[l.From] = append(s.outs[l.From], l)
		s.ins[l.To] = append(s.ins[l.To], l)
	}
}

// mix는 splitmix64의 섞기 단계다.
func mix(z uint64) uint64 {
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// rand는 요청이 생기는 간격과 캐시 적중처럼 플레이에 따라 쓰는 횟수가 달라지는 운이다.
func (s *Sim) rand(n int) int {
	s.rng += 0x9e3779b97f4a7c15
	return int(mix(s.rng) % uint64(n))
}

// 시드로 고정하는 운의 종류
const (
	luckReward = iota + 1
	luckFault
	luckTarget
	luckSpot
)

// luck은 시드와 몇 번째 일인지만으로 정하는 운이다. 같은 시드면 어떻게 플레이하든 같은 값이 나온다.
// 1판 규칙으로 시작한 판은 예전처럼 요청 흐름과 같은 운을 쓴다.
func (s *Sim) luck(kind, a, b, n int) int {
	if s.Cfg.Rules < 2 {
		return s.rand(n)
	}
	z := mix(s.Seed ^ uint64(kind)*0x9e3779b97f4a7c15)
	z = mix(z ^ uint64(a)*0xbf58476d1ce4e5b9)
	z = mix(z ^ uint64(b)*0x94d049bb133111eb)
	return int(z % uint64(n))
}

func (s *Sim) emit(e Event) {
	e.At = s.Tick
	if len(s.Events) >= maxEvents {
		s.Events = s.Events[1:]
	}
	s.Events = append(s.Events, e)
}

// Take는 쌓인 표시용 이벤트를 꺼낸다.
func (s *Sim) Take() []Event {
	e := s.Events
	s.Events = nil
	return e
}

func (s *Sim) addNode(k Kind, x, y int) *Node {
	s.nextID++
	n := &Node{ID: s.nextID, Kind: k, X: x, Y: y, Speed: 100, Index: 1}
	for _, o := range s.Nodes {
		if o.Kind == k && o.Index >= n.Index {
			n.Index = o.Index + 1
		}
	}
	n.Born = s.Tick
	s.Nodes = append(s.Nodes, n)
	s.reindex()
	return n
}

func (s *Sim) addSource(def int) {
	d := s.Cfg.Sources[def]
	s.openSource(Coming{X: d.X, Y: d.Y, Def: def})
}

func (s *Sim) openSource(c Coming) {
	n := s.addNode(Source, c.X, c.Y)
	n.Def, n.next = c.Def, 1000
	s.emit(Event{K: "source", Node: n.ID})
}

func (s *Sim) Node(id int) *Node { return s.nodeIx[id] }
func (s *Sim) Link(id int) *Link { return s.linkIx[id] }

func (s *Sim) def(k Kind) ServerDef {
	switch k {
	case DB:
		return s.Cfg.DB
	case Cache:
		return s.Cfg.Redis
	}
	return s.Cfg.API
}

// Unlocked는 그 성장 단계에서 열리는 기능을 쓸 수 있는지 답한다.
func (s *Sim) Unlocked(rank int) bool {
	r, _ := s.Rank()
	return !s.Cfg.Practice && r >= rank
}

// BuildCost는 다음 한 대를 짓는 비용이다. 첫 한 대 이후로 조금씩 오른다.
func (s *Sim) BuildCost(k Kind) int {
	d, n := s.def(k), s.count(k)
	if k != Cache {
		n = max(0, n-1) // 처음부터 있던 한 대
	}
	return d.Cost + d.CostStep*n
}

// RevenueNeed는 예산 1을 버는 데 필요한 완료 수다.
func (s *Sim) RevenueNeed() int {
	return s.Cfg.RevenueEvery + s.count(API) + s.count(DB) + s.count(Cache)
}

// RevenueMul은 구간을 넘길수록, 보상을 받을수록 늘어나는 처리 수익(퍼센트)이다.
func (s *Sim) RevenueMul() int { return 100 + s.Cfg.RevenueStep*s.Phase + s.Bonus.Revenue }

func (s *Sim) count(k Kind) int {
	c := 0
	for _, n := range s.Nodes {
		if n.Kind == k && !n.Draining {
			c++
		}
	}
	return c
}

func (s *Sim) ServerLevel(n *Node) ServerLevel {
	if n.Kind == Source {
		return ServerLevel{Queue: s.Cfg.Source.Queue}
	}
	return s.boosted(n.Kind, s.def(n.Kind).Level(n.Level))
}

// NextLevel은 한 단계 올렸을 때의 값이다.
func (s *Sim) NextLevel(n *Node) ServerLevel {
	return s.boosted(n.Kind, s.def(n.Kind).Level(n.Level+1))
}

// boosted는 보상으로 얻은 개선을 반영한다.
func (s *Sim) boosted(k Kind, lv ServerLevel) ServerLevel {
	lv.Queue += s.Bonus.Queue
	opt := 0
	switch k {
	case API:
		opt = s.Bonus.APIOpt
	case DB:
		opt = s.Bonus.DBIndex
	}
	lv.ProcMs = lv.ProcMs * (100 - min(opt, 60)) / 100
	return lv
}

// LinkRoom은 한 차선에 동시에 올라갈 수 있는 요청 수다.
func (s *Sim) LinkRoom(l *Link) int { return l.Cells * s.Cfg.Link.RoomPerCell * (l.Level + 1) }

// Area는 지금 쓸 수 있는 보드 영역이다. 구간을 넘길 때마다 넓어지고, 정해진 영역을 다 쓰면 사방으로 계속 넓어진다.
func (s *Sim) Area() Area {
	a := s.Cfg.Areas[0]
	for _, x := range s.Cfg.Areas {
		if s.Phase >= x.AtPhase {
			a = x
		}
	}
	e := s.Cfg.Endless
	if k := s.Phase - s.Cfg.Areas[len(s.Cfg.Areas)-1].AtPhase; k > 0 && e.ExpandW+e.ExpandH > 0 {
		w, h := max(a.W, min(e.MaxW, a.W+e.ExpandW*k)), max(a.H, min(e.MaxH, a.H+e.ExpandH*k))
		a.X, a.Y = a.X-(w-a.W)/2, a.Y-(h-a.H)/2
		a.W, a.H = w, h
	}
	return a
}

// Rank는 누적 완료 수로 정하는 성장 단계다.
func (s *Sim) Rank() (int, string) {
	i := 0
	for k, r := range s.Cfg.Ranks {
		if s.Completed >= r.At {
			i = k
		}
	}
	return i, s.Cfg.Ranks[i].Name
}

// P95는 최근 완료 요청의 왕복 시간(ms)이다.
func (s *Sim) P95() int {
	n := min(s.latN, len(s.lats))
	if n == 0 {
		return 0
	}
	v := slices.Clone(s.lats[:n])
	slices.Sort(v)
	return v[(n*95+99)/100-1] * TickMs
}

// RPS10은 최근 10초 동안 완료한 요청 수다.
func (s *Sim) RPS10() int {
	t := 0
	for _, v := range s.perSec {
		t += v
	}
	return t
}

func (s *Sim) LinkRate(l *Link) int { return s.linkRate(l.Level, l.WS) }

// NextLinkRate는 한 단계 올렸을 때, WSLinkRate는 WebSocket으로 전환했을 때의 전송량이다.
func (s *Sim) NextLinkRate(l *Link) int { return s.linkRate(l.Level+1, l.WS) }
func (s *Sim) WSLinkRate(l *Link) int   { return s.linkRate(l.Level, true) }

func (s *Sim) linkRate(level int, ws bool) int {
	v := s.Cfg.LinkLevel(level).PerSec * (100 + s.Bonus.LinkRate)
	if ws {
		v = v * s.Cfg.WebSocket.RatePercent / 100
	}
	return (v + 99) / 100
}

// WSTravel은 WebSocket으로 전환했을 때의 편도 틱 수다.
func (s *Sim) WSTravel(l *Link) int {
	p := s.Cfg.WebSocket.SpeedPercent
	return max(1, (l.Travel*100+p-1)/p)
}

func (s *Sim) WSCost() int {
	n := 0
	for _, l := range s.Links {
		if l.WS && !l.Draining {
			n++
		}
	}
	return s.Cfg.WebSocket.Cost + s.Cfg.WebSocket.CostStep*n
}

// CanWS는 전환할 수 있는 링크인지 답한다. 클라이언트와 서버 사이에서만 쓴다.
func (s *Sim) CanWS(l *Link) bool {
	return s.Unlocked(s.Cfg.Unlock.WebSocket) && !l.WS && !l.Draining && s.Node(l.From).Kind == Source
}

// 저장 공간

// StorageCap은 DB가 담을 수 있는 데이터 양이다.
func (s *Sim) StorageCap(n *Node) int  { return s.storageCap(n.Store) }
func (s *Sim) NextStorage(n *Node) int { return s.storageCap(n.Store + 1) }

func (s *Sim) storageCap(level int) int {
	c := s.Cfg.Storage.Base
	for range level {
		c = c * (100 + s.Cfg.Storage.GrowPercent) / 100
	}
	return c * (100 + s.Bonus.Storage) / 100
}

func (s *Sim) StorageCost(n *Node) int { return s.Cfg.Storage.Cost + s.Cfg.Storage.CostStep*n.Store }

// Pace는 장애와 저장 공간을 모두 반영한 처리 속도 퍼센트다.
func (s *Sim) Pace(n *Node) int {
	if n.Kind == DB && n.Data > 0 && n.Data >= s.StorageCap(n) {
		return n.Speed * s.Cfg.Storage.SlowPercent / 100
	}
	return n.Speed
}

// store는 처리한 요청 하나를 저장한다. 경고선과 상한을 넘는 순간을 알린다.
func (s *Sim) store(n *Node) {
	limit := s.StorageCap(n)
	if n.Data >= limit {
		return
	}
	n.Data++
	if n.Data == limit {
		s.emit(Event{K: "store", Node: n.ID, A: 100})
	} else if warn := limit * s.Cfg.Storage.WarnPercent / 100; n.Data == warn {
		s.emit(Event{K: "store", Node: n.ID, A: s.Cfg.Storage.WarnPercent})
	}
}

// spread는 데이터를 모든 DB에 저장 공간에 비례해 다시 나눈다. 샤드가 늘거나 줄 때 부른다.
func (s *Sim) spread(extra int) {
	total, room := extra, 0
	for _, n := range s.Nodes {
		if n.Kind == DB && !n.Draining {
			total += n.Data
			room += s.StorageCap(n)
		}
	}
	for _, n := range s.Nodes {
		if n.Kind == DB && !n.Draining && room > 0 {
			limit := s.StorageCap(n)
			n.Data = min(limit, total*limit/room)
		}
	}
}

// Surge는 오래 버틸수록 커지는 전체 트래픽 배율(퍼센트)이다.
func (s *Sim) Surge(tick int) int {
	e := s.Cfg.Endless
	t := tick - e.SurgeFromMin*60*TPS
	if e.SurgePerMin == 0 || t <= 0 {
		return 100
	}
	m := t * 100 / (60 * TPS) // 분의 100배
	return 100 + (e.SurgePerMin*m+m*m/200)/100
}

// Rate는 클라이언트의 현재 초당 요청 수(1000배)다.
func (s *Sim) Rate(n *Node, tick int) int { return s.baseRate(n, tick) * s.Surge(tick) / 100 }

// baseRate는 설정 지점 사이를 부드럽게 잇는다.
func (s *Sim) baseRate(n *Node, tick int) int {
	if s.Cfg.Practice {
		return s.load
	}
	e := s.Cfg.Endless
	if n.Def < 0 {
		t := tick - n.Born
		ramp := min(t, e.RampMin*60*TPS)
		return e.RateStart + (e.RatePerMin*ramp+e.GrowthPerMin*(t-ramp))/(60*TPS)
	}
	pts := s.Cfg.Sources[n.Def].Rate
	for i := len(pts) - 1; i >= 0; i-- {
		a := pts[i]
		if tick < a[0]*TPS {
			continue
		}
		if i == len(pts)-1 {
			return a[1] + e.GrowthPerMin*(tick-a[0]*TPS)/(60*TPS)
		}
		b := pts[i+1]
		return a[1] + (b[1]-a[1])*(tick-a[0]*TPS)/((b[0]-a[0])*TPS)
	}
	return 0
}

func dist(ax, ay, bx, by int) float64 { return math.Hypot(float64(ax-bx), float64(ay-by)) }

// cells는 두 칸 사이 거리를 올림한 칸 수다.
func cells(ax, ay, bx, by int) int { return int(math.Ceil(dist(ax, ay, bx, by) - 1e-9)) }

func (s *Sim) LinkCost(a, b *Node) int {
	return s.Cfg.Link.BaseCost + s.Cfg.Link.PerCell*cells(a.X, a.Y, b.X, b.Y)
}

// Advance는 n틱까지 진행한다. 일시 정지, 보상 선택 대기, 종료 상태에서는 멈춘다.
func (s *Sim) Advance(n int) int {
	done := 0
	for ; done < n && !s.Paused && s.Pending == nil && s.Over == ""; done++ {
		s.step()
	}
	return done
}

func (s *Sim) step() {
	s.Tick++
	if s.Tick%TPS == 1 {
		s.gainLeft, s.lossLeft = s.Cfg.Health.MaxGainPerSec, s.Cfg.Health.MaxLossPerSec
		s.perSec[s.Tick/TPS%len(s.perSec)] = 0
	}
	s.stepGrowth()
	s.stepFaults()
	s.stepSources()
	s.stepArrivals()
	s.stepJobs()
	s.stepDispatch()
	s.stepStart()
	s.stepDeadlines()
	s.stepCleanup()
	s.stepEnd()
}

// stepGrowth는 클라이언트 예고와 등장, 마라톤 장애 예약을 처리한다.
func (s *Sim) stepGrowth() {
	for i, d := range s.Cfg.Sources {
		if d.AppearSec > 0 && s.Tick == (d.AppearSec-d.NoticeSec)*TPS {
			s.Coming = append(s.Coming, Coming{d.X, d.Y, d.AppearSec * TPS, i})
		}
	}
	e := s.Cfg.Endless
	if e.SourceEverySec > 0 && s.Tick == s.sourceAt-e.SourceNoticeSec*TPS {
		if x, y, ok := s.spot(); ok {
			s.Coming = append(s.Coming, Coming{x, y, s.sourceAt, -1})
		}
		s.sourceN++
		s.sourceAt += max(e.SourceMinEverySec, e.SourceEverySec-s.sourceN) * TPS
	}
	keep := s.Coming[:0]
	for _, c := range s.Coming {
		if c.At > s.Tick {
			keep = append(keep, c)
			continue
		}
		s.openSource(c)
	}
	s.Coming = keep

	if e.FaultEverySec > 0 && s.Tick == s.faultAt-30*TPS {
		types := []FaultDef{
			{Type: "api_slow", NoticeSec: 14, DurationSec: 18, Percent: 40},
			{Type: "link_block", NoticeSec: 14, DurationSec: 16},
			{Type: "api_down", NoticeSec: 16, DurationSec: 14},
		}
		d := types[s.luck(luckFault, s.faultN, 0, len(types))]
		d.AtSec = s.faultAt / TPS
		s.addFault(d)
		s.faultN++
		s.faultAt += max(e.FaultMinEverySec, e.FaultEverySec-s.faultN*2) * TPS
	}
	// 끝난 장애는 최근 것만 남긴다.
	if len(s.Faults) > 24 {
		for i, f := range s.Faults {
			if f.State == "ended" || f.State == "canceled" {
				s.Faults = append(s.Faults[:i:i], s.Faults[i+1:]...)
				break
			}
		}
	}
}

// spot은 새 클라이언트 자리를 고른다. 후보 중 다른 노드와 가장 멀리 떨어진 곳이다.
// 후보는 시드로 정하지만, 지어 둔 노드를 피해 고르므로 플레이에 따라 다른 후보가 뽑힐 수 있다.
func (s *Sim) spot() (int, int, bool) {
	a := s.Area()
	bx, by, best := 0, 0, 0.0
	try := func(x, y int, near float64) {
		gap := math.Inf(1)
		for _, n := range s.Nodes {
			gap = math.Min(gap, dist(x, y, n.X, n.Y))
		}
		for _, c := range s.Coming {
			gap = math.Min(gap, dist(x, y, c.X, c.Y))
		}
		// 너무 외딴 곳보다 기존 서비스망 근처를 고른다.
		if gap >= near && gap <= 9 && gap > best {
			bx, by, best = x, y, gap
		}
	}
	for i := range 32 {
		try(a.X+1+s.luck(luckSpot, s.sourceN, 2*i, max(1, a.W-2)), a.Y+1+s.luck(luckSpot, s.sourceN, 2*i+1, max(1, a.H-2)), 3)
	}
	// 빈자리를 메워 새 클라이언트를 막을 수 없게, 못 찾으면 모든 칸에서 좁은 곳까지 찾는다.
	if best == 0 {
		for x := a.X + 1; x < a.X+a.W-1; x++ {
			for y := a.Y + 1; y < a.Y+a.H-1; y++ {
				try(x, y, float64(s.Cfg.MinNodeGap))
			}
		}
	}
	return bx, by, best > 0
}

func (s *Sim) stat(t string) *FaultStat {
	for i := range s.Stats {
		if s.Stats[i].Type == t {
			return &s.Stats[i]
		}
	}
	s.Stats = append(s.Stats, FaultStat{Type: t})
	return &s.Stats[len(s.Stats)-1]
}

func (s *Sim) stepFaults() {
	for _, n := range s.Nodes {
		n.Speed = 100
	}
	for _, l := range s.Links {
		l.Blocked = false
	}
	for _, f := range s.Faults {
		switch f.State {
		case "pending":
			if s.Tick >= f.StartAt-f.Def.NoticeSec*TPS {
				s.noticeFault(f)
			}
		case "notice":
			if s.Tick >= f.StartAt {
				if !s.faultTargetAlive(f) {
					f.State = "canceled"
					s.emit(Event{K: "fault", A: f.ID, Text: "canceled"})
					break
				}
				f.State = "active"
				s.stat(f.Def.Type).Count++
				s.emit(Event{K: "fault", A: f.ID, Text: "active"})
			}
		case "active":
			if s.Tick >= f.EndAt || !s.faultTargetAlive(f) {
				f.State = "ended"
				s.emit(Event{K: "fault", A: f.ID, Text: "ended"})
			}
		}
		if f.State != "active" {
			continue
		}
		switch f.Def.Type {
		case "api_slow":
			s.Node(f.Node).Speed = f.Def.Percent
		case "api_down":
			s.Node(f.Node).Speed = 0
		case "link_block":
			s.Link(f.Link).Blocked = true
		}
	}
}

func (s *Sim) faultTargetAlive(f *Fault) bool {
	if f.Def.Type == "link_block" {
		l := s.Link(f.Link)
		return l != nil && !l.Draining
	}
	n := s.Node(f.Node)
	return n != nil && !n.Draining
}

// noticeFault는 많이 쓴 순서로 대상을 고른다. 정해진 초반 장애는 1위, 그 뒤로는 3위까지 중 하나다.
func (s *Sim) noticeFault(f *Fault) {
	type cand struct{ id, used int }
	var list []cand
	if f.Def.Type == "link_block" {
		for _, l := range s.Links {
			if !l.Draining && s.Node(l.From).Kind == Source {
				list = append(list, cand{l.ID, l.Passed})
			}
		}
	} else {
		for _, n := range s.Nodes {
			if n.Kind == API && !n.Draining {
				list = append(list, cand{n.ID, n.Done})
			}
		}
	}
	f.State = "canceled"
	if len(list) > 0 {
		slices.SortStableFunc(list, func(a, b cand) int { return b.used - a.used })
		top := 1
		if f.ID > len(s.Cfg.Faults) {
			top = min(3, len(list))
		}
		f.State = "notice"
		if id := list[s.luck(luckTarget, f.ID, 0, top)].id; f.Def.Type == "link_block" {
			f.Link = id
		} else {
			f.Node = id
		}
	}
	s.emit(Event{K: "fault", A: f.ID, Text: f.State})
}

func (s *Sim) stepSources() {
	if s.Tick < s.Cfg.Request.StartSec*TPS {
		return
	}
	for _, n := range s.Nodes {
		if n.Kind != Source {
			continue
		}
		n.acc += s.Rate(n, s.Tick) * TickMs / 1000
		for n.acc >= n.next {
			n.acc -= n.next
			n.next = 750 + s.rand(501)
			s.spawn(n)
		}
	}
}

func (s *Sim) spawn(n *Node) {
	s.nextReq++
	r := &Request{ID: s.nextReq, State: Waiting, Source: n.ID, Born: s.Tick, Deadline: s.Tick + (s.Cfg.Request.DeadlineSec+s.Bonus.Deadline)*TPS}
	s.Spawned++
	s.Active++
	n.Spawned++
	if len(n.Queue) >= s.Cfg.Source.Queue || s.Active > s.Cfg.Request.MaxActive {
		reason := FailOverflow
		if len(n.Queue) > 0 {
			if _, why := s.route(n, n.Queue[0]); why == FailNoRoute || why == FailFault {
				reason = why
			}
		}
		s.fail(r, reason, n, nil)
		return
	}
	n.Queue = append(n.Queue, r)
}

// stepArrivals는 차선 맨 앞 요청부터 들여보낸다. 서버가 꽉 차 있으면 링크 위에서 기다린다.
func (s *Sim) stepArrivals() {
	for _, l := range s.Links {
		l.Jam = false
		for d := range l.Lanes {
			ln := &l.Lanes[d]
			if len(ln.Moving) == 0 || ln.Moving[0].Arrive > s.Tick {
				continue
			}
			r := ln.Moving[0]
			if d == Fwd {
				n := s.Node(l.To)
				if n.Kind == r.want() && len(n.Queue) >= s.ServerLevel(n).Queue {
					l.Jam = true
					continue
				}
			}
			ln.Moving[0] = nil
			ln.Moving = ln.Moving[1:]
			if d == Fwd {
				s.arrive(r, s.Node(l.To))
			} else {
				s.comeBack(r, l)
			}
		}
	}
}

// comeBack은 응답이 지나온 링크 하나를 되돌아온 것을 처리한다.
func (s *Sim) comeBack(r *Request, l *Link) {
	r.Path = r.Path[:len(r.Path)-1]
	l.Pending--
	n := s.Node(l.From)
	s.emit(Event{K: "in", Req: r.ID, Node: n.ID})
	if n.Kind == API {
		r.Stage, r.State = BackToUser, Sending
		n.Out = append(n.Out, r)
		return
	}
	s.complete(r, n, l.WS)
}

func clearTail[T any](v []*T, from int) {
	for i := from; i < len(v); i++ {
		v[i] = nil
	}
}

func stageKind(stage int) Kind {
	if stage == ToAPI {
		return API
	}
	return DB
}

func (s *Sim) arrive(r *Request, n *Node) {
	if n.Kind != r.want() || r.Hops > s.Cfg.Request.MaxHops {
		s.fail(r, FailNoRoute, n, nil)
		return
	}
	s.emit(Event{K: "in", Req: r.ID, Node: n.ID})
	r.State = Waiting
	n.Queue = append(n.Queue, r)
}

func (s *Sim) stepJobs() {
	for _, n := range s.Nodes {
		if n.Kind == Source {
			continue
		}
		pace := s.Pace(n)
		keep := n.Jobs[:0]
		for _, j := range n.Jobs {
			if !j.Done {
				j.Left -= pace
				if j.Left <= 0 {
					j.Done = true
					j.Req.Stage++
					j.Req.State = Sending
					n.Done++
					if n.Kind == API && s.Bonus.Cache > 0 && s.rand(100) < s.Bonus.Cache {
						// 캐시에 있으면 DB를 거치지 않고 바로 응답한다.
						j.Req.Stage = BackToUser
						n.Out = append(n.Out, j.Req)
						s.emit(Event{K: "cache", Req: j.Req.ID, Node: n.ID})
						continue
					}
					if n.Kind == API {
						if hit := s.redisHit(n); hit > 0 {
							j.Req.Hit = s.rand(100) < hit
						}
					}
					if n.Kind == DB && s.Unlocked(s.Cfg.Unlock.Storage) {
						s.store(n)
					}
				}
			}
			keep = append(keep, j)
		}
		clearTail(n.Jobs, len(keep))
		n.Jobs = keep
	}
}

// redisHit은 이 서버와 이어진 Redis 중 가장 높은 적중률이다.
func (s *Sim) redisHit(n *Node) int {
	hit := 0
	for _, l := range s.outs[n.ID] {
		if s.redisLink(l, true) {
			hit = max(hit, s.ServerLevel(s.Node(l.To)).Hit)
		}
	}
	return hit
}

func (s *Sim) redisLink(l *Link, faults bool) bool {
	to := s.Node(l.To)
	return !l.Draining && !(faults && l.Blocked) && to != nil && to.Kind == Cache && !to.Draining
}

func (s *Sim) complete(r *Request, n *Node, ws bool) {
	r.State, r.Stage = Completed, Finished
	s.Active--
	s.Completed++
	n.Done++
	s.perSec[s.Tick/TPS%len(s.perSec)]++
	s.lats[s.latN%len(s.lats)] = s.Tick - r.Born
	s.latN++
	s.Score += s.Cfg.ScorePerComplete
	// 처리 수익. 서버가 많을수록 운영비가 들어 예산 1을 버는 데 더 많은 완료가 필요하다.
	if s.Cfg.RevenueEvery > 0 {
		v := s.RevenueMul()
		if ws {
			v = v * s.Cfg.WebSocket.RevenuePercent / 100
		}
		s.revenue += v
		if need := s.RevenueNeed() * 100; s.revenue >= need {
			s.Budget += s.revenue / need
			s.revenue %= need
		}
	}
	gain := min(s.Cfg.Health.SuccessGain, s.gainLeft, s.Cfg.Health.Max-s.Health)
	s.Health += gain
	s.gainLeft -= gain
	s.emit(Event{K: "done", Req: r.ID, Node: n.ID, A: (s.Tick - r.Born) * TickMs})
}

func (s *Sim) fail(r *Request, reason int, n *Node, l *Link) {
	r.State = Failed
	s.Active--
	s.Failures[reason]++
	for _, id := range r.Path {
		if p := s.Link(id); p != nil {
			p.Pending--
		}
	}
	r.Path = nil
	e := Event{K: "fail", Req: r.ID, A: reason, B: (s.Tick - r.Born) * TickMs}
	if n != nil {
		n.Fail[reason]++
		e.Node = n.ID
	}
	if l != nil {
		l.Fail[reason]++
		e.Link = l.ID
	}
	penalty := s.Cfg.Health.FailPenalty
	for _, f := range s.Faults {
		if f.State == "active" {
			f.FailedIn++
			s.stat(f.Def.Type).Failed++
			penalty = s.Cfg.Health.FailPenalty * s.Cfg.Health.FaultPercent / 100
		}
	}
	// 실패가 한꺼번에 몰려도 1초에 잃는 양에는 상한이 있어, 손쓸 틈 없이 무너지지 않는다. 0이면 상한이 없다.
	if s.Cfg.Health.MaxLossPerSec > 0 {
		penalty = min(penalty, s.lossLeft)
		s.lossLeft -= penalty
	}
	s.Health -= penalty
	if s.Tick < s.Cfg.Health.LearnSeconds*TPS && s.Health < s.Cfg.Health.LearnFloor {
		s.Health = s.Cfg.Health.LearnFloor
	}
	s.emit(e)
}

// usable은 구조만 보고 다음 단계 이후까지 이어지는 경로인지 확인한다. faults가 참이면 장애도 반영한다.
func (s *Sim) usable(l *Link, stage int, faults bool) bool {
	if l.Draining || (faults && l.Blocked) {
		return false
	}
	to := s.Node(l.To)
	if to == nil || to.Draining || to.Kind != stageKind(stage) || (faults && to.Speed == 0) {
		return false
	}
	if stage >= ToDB {
		return true
	}
	for _, o := range s.outs[to.ID] {
		if s.usable(o, stage+1, faults) {
			return true
		}
	}
	return false
}

// eta는 이동 시간과 도착지에서 기다릴 시간을 더한 틱 수다.
func (s *Sim) eta(l *Link) int {
	to := s.Node(l.To)
	lv := s.ServerLevel(to)
	ahead := len(to.Queue) + len(to.Jobs)
	for _, o := range s.ins[to.ID] {
		ahead += len(o.Lanes[Fwd].Moving)
	}
	proc := lv.ProcMs * 100 / TickMs / max(s.Pace(to), 10)
	return l.Travel + ahead*proc/lv.Slots
}

const routeWait = -1

// route는 지금 보낼 링크를 고른다. 없으면 이유(경로 없음, 장애, 전송 대기)를 돌려준다.
func (s *Sim) route(n *Node, r *Request) (*Link, int) {
	var best, bestReady *Link
	bestEta, readyEta := 0, 0
	structural := false
	if r.Hit && s.redisHit(n) == 0 {
		r.Hit = false // 그 사이 Redis로 가는 길이 사라졌으면 DB로 간다
	}
	ok := func(l *Link, faults bool) bool {
		if r.want() == Cache {
			return s.redisLink(l, faults)
		}
		return s.usable(l, r.Stage, faults)
	}
	for _, l := range s.outs[n.ID] {
		if ok(l, false) {
			structural = true
		}
		if !ok(l, true) {
			continue
		}
		e := s.eta(l)
		if best == nil || e < bestEta {
			best, bestEta = l, e
		}
		if l.open(s, Fwd) && (bestReady == nil || e < readyEta) {
			bestReady, readyEta = l, e
		}
	}
	switch {
	case bestReady != nil:
		return bestReady, 0
	case best != nil:
		return best, routeWait
	case structural:
		return nil, FailFault
	}
	return nil, FailNoRoute
}

func (l *Link) open(s *Sim, d int) bool {
	return !l.Blocked && l.Lanes[d].credit >= 1000 && len(l.Lanes[d].Moving) < s.LinkRoom(l)
}

func (s *Sim) send(l *Link, d int, r *Request) {
	ln := &l.Lanes[d]
	ln.credit -= 1000
	ln.ring[s.Tick%TPS]++
	if d == Fwd {
		l.Passed++
		l.Pending++
		r.Path = append(r.Path, l.ID)
	}
	r.State, r.Depart, r.Arrive = Moving, s.Tick, s.Tick+l.Travel
	r.Hops++
	ln.Moving = append(ln.Moving, r)
	s.emit(Event{K: "go", Req: r.ID, Link: l.ID, A: r.Depart, B: r.Arrive, S: r.Stage})
}

func (s *Sim) backLink(r *Request) *Link { return s.Link(r.Path[len(r.Path)-1]) }

func (s *Sim) stepDispatch() {
	for _, l := range s.Links {
		per := s.LinkRate(l) * TickMs
		for d := range l.Lanes {
			ln := &l.Lanes[d]
			ln.credit = min(ln.credit+per, 1000+per)
			ln.ring[s.Tick%TPS] = 0
		}
		l.Waiting = 0
	}
	for _, n := range s.Nodes {
		if n.Kind == Source {
			for len(n.Queue) > 0 {
				l, why := s.route(n, n.Queue[0])
				if why == routeWait {
					l.Waiting += len(n.Queue)
				}
				if l == nil || why == routeWait {
					break
				}
				s.send(l, Fwd, n.Queue[0])
				n.Queue[0] = nil
				n.Queue = n.Queue[1:]
			}
			continue
		}
		// 응답을 먼저 내보낸다. 돌아가는 차선은 서버가 꽉 차도 막히지 않는다.
		keepOut := n.Out[:0]
		for _, r := range n.Out {
			if l := s.backLink(r); l.open(s, Back) {
				s.send(l, Back, r)
				continue
			}
			keepOut = append(keepOut, r)
		}
		clearTail(n.Out, len(keepOut))
		n.Out = keepOut

		n.Stuck = 0
		keep := n.Jobs[:0]
		for _, j := range n.Jobs {
			if j.Done {
				if n.Kind != API {
					if l := s.backLink(j.Req); l.open(s, Back) {
						s.send(l, Back, j.Req)
						continue
					} else {
						l.Waiting++
					}
				} else {
					l, why := s.route(n, j.Req)
					if l != nil && why != routeWait {
						s.send(l, Fwd, j.Req)
						continue
					}
					if why == routeWait {
						l.Waiting++
					}
				}
				n.Stuck++
			}
			keep = append(keep, j)
		}
		clearTail(n.Jobs, len(keep))
		n.Jobs = keep
	}
	for _, l := range s.Links {
		for d := range l.Lanes {
			ln := &l.Lanes[d]
			ln.Used = 0
			for _, v := range ln.ring {
				ln.Used += v
			}
		}
	}
}

func (s *Sim) stepStart() {
	for _, n := range s.Nodes {
		if n.Kind == Source || n.Speed == 0 {
			continue
		}
		lv := s.ServerLevel(n)
		for len(n.Jobs) < lv.Slots && len(n.Queue) > 0 {
			r := n.Queue[0]
			n.Queue[0] = nil
			n.Queue = n.Queue[1:]
			r.State = Processing
			n.Jobs = append(n.Jobs, &Job{Req: r, Start: s.Tick, Left: lv.ProcMs * 100 / TickMs})
		}
	}
}

func (s *Sim) stepDeadlines() {
	for _, n := range s.Nodes {
		keep := n.Queue[:0]
		for _, r := range n.Queue {
			if r.Deadline > s.Tick {
				keep = append(keep, r)
				continue
			}
			reason := FailDeadline
			if n.Speed == 0 {
				reason = FailFault
			} else if n.Kind == Source {
				if _, why := s.route(n, r); why == FailNoRoute || why == FailFault {
					reason = why
				}
			}
			s.fail(r, reason, n, nil)
		}
		clearTail(n.Queue, len(keep))
		n.Queue = keep

		jobs := n.Jobs[:0]
		for _, j := range n.Jobs {
			if j.Req.Deadline > s.Tick {
				jobs = append(jobs, j)
				continue
			}
			reason := FailDeadline
			if n.Speed == 0 {
				reason = FailFault
			} else if j.Done && n.Kind == API {
				if _, why := s.route(n, j.Req); why == FailNoRoute || why == FailFault {
					reason = why
				}
			}
			s.fail(j.Req, reason, n, nil)
		}
		clearTail(n.Jobs, len(jobs))
		n.Jobs = jobs

		outs := n.Out[:0]
		for _, r := range n.Out {
			if r.Deadline > s.Tick {
				outs = append(outs, r)
				continue
			}
			reason := FailDeadline
			if s.backLink(r).Blocked {
				reason = FailFault
			}
			s.fail(r, reason, n, nil)
		}
		clearTail(n.Out, len(outs))
		n.Out = outs
	}
	for _, l := range s.Links {
		for d := range l.Lanes {
			ln := &l.Lanes[d]
			keep := ln.Moving[:0]
			for _, r := range ln.Moving {
				if r.Deadline > s.Tick {
					keep = append(keep, r)
					continue
				}
				reason := FailDeadline
				if d == Fwd && s.Node(l.To).Speed == 0 {
					reason = FailFault
				}
				s.fail(r, reason, nil, l)
			}
			clearTail(ln.Moving, len(keep))
			ln.Moving = keep
		}
	}
}

// stepCleanup은 왕복 중인 요청이 모두 돌아온 링크와 서버만 지운다.
func (s *Sim) stepCleanup() {
	for _, n := range s.Nodes {
		n.Pending = len(n.Queue) + len(n.Jobs) + len(n.Out)
	}
	for _, l := range s.Links {
		s.Node(l.To).Pending += l.Pending
	}
	for _, l := range s.Links {
		if n := s.Node(l.From); n.Draining && n.Pending == 0 {
			l.Draining = true
		}
	}
	before := len(s.Links) + len(s.Nodes)
	links := s.Links[:0]
	for _, l := range s.Links {
		if l.Draining && l.Pending == 0 {
			continue
		}
		links = append(links, l)
	}
	clearTail(s.Links, len(links))
	s.Links = links
	nodes, moved := s.Nodes[:0], 0
	for _, n := range s.Nodes {
		if n.Draining && n.Pending == 0 && !s.linked(n.ID) {
			moved += n.Data
			continue
		}
		nodes = append(nodes, n)
	}
	clearTail(s.Nodes, len(nodes))
	s.Nodes = nodes
	if moved > 0 {
		s.spread(moved)
	}
	if len(s.Links)+len(s.Nodes) != before {
		s.reindex()
	}
}

func (s *Sim) linked(id int) bool {
	for _, l := range s.Links {
		if l.From == id || l.To == id {
			return true
		}
	}
	return false
}

func (s *Sim) stepEnd() {
	wait := 0
	for _, n := range s.Nodes {
		wait += len(n.Queue)
	}
	for _, l := range s.Links {
		if l.Jam {
			wait += len(l.Lanes[Fwd].Moving)
		}
	}
	s.PeakWait = max(s.PeakWait, wait)
	if s.Health <= 0 {
		s.Health = 0
		s.Over = "down"
		return
	}
	s.PeakRPS = max(s.PeakRPS, s.RPS10())
	if s.Tick < s.PhaseEnd {
		return
	}
	s.Phase++
	e, list := s.Cfg.Endless, s.Cfg.PhaseIncome
	income := list[min(s.Phase, len(list))-1] + max(0, s.Phase-len(list))*e.IncomeStep
	held := s.Budget
	interest := min(income, held*s.Cfg.InterestPercent/100)
	s.Budget += income + interest
	s.PhaseAt = s.Tick
	s.PhaseEnd = s.Tick + min(max(e.PhaseMaxSec, s.Cfg.PhaseSeconds), s.Cfg.PhaseSeconds+s.Phase*e.PhaseStepSec)*TPS
	// 후보 순서는 시드와 구간만으로 섞고, 더 좋아질 수 없는 보상은 그 자리에서 뺀다.
	// 그래서 같은 시드면 빠진 것 말고는 플레이와 상관없이 같은 카드가 나온다.
	order := slices.Clone(s.Cfg.Rewards)
	if s.Cfg.Rules >= 2 {
		for i := len(order) - 1; i > 0; i-- {
			j := s.luck(luckReward, s.Phase, i, i+1)
			order[i], order[j] = order[j], order[i]
		}
	}
	var pool []RewardDef
	for _, r := range order {
		if (r.ID == "cache" && s.Bonus.Cache >= 60) || (r.ID == "apiOpt" && s.Bonus.APIOpt >= 60) || (r.ID == "dbIndex" && s.Bonus.DBIndex >= 60) ||
			(r.ID == "deadline" && s.Bonus.Deadline >= 16) || (r.ID == "health" && s.Health >= s.Cfg.Health.Max) || (r.ID == "storage" && !s.Unlocked(s.Cfg.Unlock.Storage)) {
			continue
		}
		r.Amount, r.Step = r.Amount+r.Step*s.Phase/10, 0
		pool = append(pool, r)
	}
	// 구간이 오를수록 선택지가 둘에서 다섯까지 늘고, 넷부터는 두 개를 고른다.
	n, left := min(s.Phase+1, 5, len(pool)), 1
	if s.Cfg.Rules < 2 {
		// 1판 규칙: 남은 후보에서 요청 흐름과 같은 운으로 골랐다.
		for i := range n {
			j := i + s.rand(len(pool)-i)
			pool[i], pool[j] = pool[j], pool[i]
		}
	}
	if n >= 4 {
		left = 2
	}
	s.Pending = &Reward{Phase: s.Phase, Income: income, Held: held, Interest: interest, Options: pool[:n], Taken: make([]bool, n), Left: left}
}

// Routed는 클라이언트에서 완료까지 이어지는 길이 있는지 구조만 보고 답한다.
func (s *Sim) Routed(n *Node) bool {
	for _, l := range s.outs[n.ID] {
		if s.usable(l, ToAPI, false) {
			return true
		}
	}
	return false
}
