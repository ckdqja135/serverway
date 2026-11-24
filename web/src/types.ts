export const SOURCE = 0, API = 1, DB = 2
export interface Job { id: number; s: number; e: number; d?: boolean }
export interface GNode {
  id: number; kind: number; index: number; x: number; y: number; level: number; slots: number; procMs: number; limit: number
  queue: number; out: number; pending: number; jobs: Job[]; stuck: number; draining: boolean; speed: number; state: 'ok' | 'busy' | 'full' | 'slow' | 'down'
  done: number; fail: number[]; spawned: number; rate: number; nextRate: number; upgrade: number
  next?: { slots: number; procMs: number; limit: number }; refund: number; routed: boolean
}
export interface GLink {
  id: number; from: number; to: number; level: number; cap: number; used: number; waiting: number; travel: number; room: number
  moving: [number, number]; pending: number; passed: number; fail: number[]; state: 'ok' | 'busy' | 'full' | 'jam' | 'blocked'; draining: boolean
  upgrade: number; nextCap: number; refund: number
}
export interface Fault { id: number; type: 'api_slow' | 'api_down' | 'link_block'; state: string; node?: number; link?: number; startAt: number; endAt: number; percent: number; failedIn: number }
export interface Reward { phase: number; income: number; options: { id: string; amount: number }[] }
export interface Area { x: number; y: number; w: number; h: number }
export interface Head {
  tick: number; score: number; health: number; budget: number; paused: boolean; started: boolean; speed: number; over: string; phase: number; phaseAt: number; phaseEnd: number
  apiCost: number; dbCost: number; revenueNeed: number; rank: number; rankName: string; nextRank: number; surge: number; servers: number
  spawned: number; completed: number; failures: number[]; active: number; reward: Reward | null; faults: Fault[]
  upcoming: { x: number; y: number; at: number }[]; rps10: number; p95: number; area: Area; cache: number; deadline: number; pauseReason: string
}
export interface Config {
  practice: boolean; tps: number; healthMax: number; score: number; linkBase: number; linkPerCell: number
  minNodeGap: number; maxServers: number; learnSeconds: number; startSec: number; sources: { x: number; y: number }[]; refundPercent: number; failPenalty: number; faultPercent: number
}
export interface Result {
  over: string; seconds: number; score: number; completed: number; failures: number[]; health: number; spent: number; peakWait: number; apis: number; links: number
  faults: { type: Fault['type']; count: number; failed: number }[]; advice: string; seed: number; p95: number; peakRps: number; rank: string; sources: number; dbs: number
}
export interface GEvent { k: string; at: number; r?: number; n?: number; l?: number; a?: number; b?: number; s?: number; t?: string }
export interface Ack { seq: number; ok: boolean; error?: string; tick: number; id?: number }
export type Selection = { type: 'node' | 'link'; id: number } | null
export type Tool = 'connect' | 'api' | 'db'

export interface Best { score: number; seconds: number; rank: string }
export const loadBest = (): Best | null => { try { return JSON.parse(localStorage.getItem('serverway.best') ?? 'null') } catch { return null } }
export const clockText = (s: number) => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, '0')}`

// 실패 사유 순서는 서버와 같다: 경로 없음, 대기열 초과, 기한 초과, 장애
export const FAIL = [
  { code: 502, name: '경로 없음', http: 'Bad Gateway' },
  { code: 503, name: '대기열 초과', http: 'Service Unavailable' },
  { code: 504, name: '처리 기한 초과', http: 'Gateway Timeout' },
  { code: 500, name: '장애로 처리 불가', http: 'Server Error' },
]
export const FAULT_TEXT: Record<Fault['type'], { title: string; effect: (f: Fault) => string; tip: string }> = {
  api_slow: { title: 'GC 지연', effect: (f) => `처리 속도 ${f.percent}%로 저하`, tip: '다른 API로 나눠 보낼 링크를 만들어 두세요' },
  api_down: { title: '배포 실패', effect: () => '재시작하는 동안 처리 중단', tip: '다른 API를 키우고 우회 링크를 준비하세요' },
  link_block: { title: '회선 단선', effect: () => '새 요청과 응답이 지나갈 수 없음', tip: '같은 발생지에서 다른 API로 링크를 하나 더 만드세요' },
}
export const nodeName = (n: GNode) => (n.kind === SOURCE ? `발생지 ${n.index}` : n.kind === API ? `API ${n.index}` : `DB ${n.index}`)
