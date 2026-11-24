import { FAIL, type Ack, type Config, type GEvent, type GLink, type GNode, type Head, type Result } from './types'
import { sfx, type Sfx } from './sound'
import { host, type Wire } from './wire'

export interface Dot { id: number; link: number; lane: 0 | 1; dep: number; arr: number; stage: number; strand: number; p: number }
export interface Fx { kind: 'done' | 'fail' | 'cache' | 'spawn' | 'up'; x: number; y: number; at: number; text?: string }
export interface LogLine { id: number; at: number; code: number; text: string }
export type Status = 'connecting' | 'open' | 'lost' | 'ended'

const MAX_FX = 120
const DID: Record<string, Sfx> = { link: 'link', build: 'build', upgradeNode: 'up', upgradeLink: 'up', removeLink: 'remove', removeNode: 'remove' } // 명령이 받아들여졌을 때의 소리
const MAX_LOG = 60

function turn(o: { x: number; y: number; w?: number; h?: number }) {
  [o.x, o.y] = [o.y, o.x]
  if (o.w !== undefined && o.h !== undefined) [o.w, o.h] = [o.h, o.w]
}
const turnHead = (h: Head) => { turn(h.area); h.upcoming.forEach(turn) }

// GameClient는 서버의 기준 상태를 받아 두는 곳이다. 점수나 성공 여부를 스스로 계산하지 않는다.
export class GameClient {
  status: Status = 'connecting'
  epoch = 0
  seed = 0
  config!: Config
  head!: Head
  nodes = new Map<number, GNode>()
  links = new Map<number, GLink>()
  result: Result | null = null
  lanes = new Map<string, Dot[]>()
  fx: Fx[] = []
  logs: LogLine[] = []
  notice: { text: string; bad: boolean; id: number } | null = null
  rt = 0
  version = 0
  resets = 0
  ready = false
  grew = false // 이번 구간에서 판이 넓어졌는지
  flip = false // 세로 화면에서는 x와 y를 바꿔 보여 준다
  dotPos: (d: Dot) => { x: number; y: number } | null = () => null

  private wire?: Wire
  private seq = 0
  private queue: GEvent[] = []
  private where = new Map<number, Dot>()
  private strands = new Map<string, number>()
  private listeners = new Set<() => void>()
  private waits = new Map<number, (a: Ack) => void>()
  private retry = 0
  private closed = false
  private noticeId = 0
  private logId = 0

  constructor(readonly id: string) { this.open() }

  static async create(seed?: number, practice = false): Promise<string> {
    return host.create(seed ?? 0, practice)
  }
  static exists(id: string) { return host.exists(id) }

  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn) } }
  getVersion = () => this.version
  private changed() { this.version++; this.listeners.forEach((f) => f()) }

  close() { this.closed = true; clearTimeout(this.retry); this.wire?.close() }

  private open() {
    const wire = host.open(this.id, (m) => { if (this.wire === wire) this.receive(m) }, (ended) => {
      if (this.closed || this.wire !== wire) return
      this.status = ended ? 'ended' : 'lost'
      this.changed()
      if (ended) return
      this.retry = window.setTimeout(async () => {
        const ok = await GameClient.exists(this.id).catch(() => null)
        if (this.closed) return
        if (ok === false) { this.status = 'ended'; this.changed() } else this.open()
      }, 1200)
    })
    this.wire = wire
  }

  send(type: string, body: Record<string, number | boolean | string> = {}): Promise<Ack> {
    if (this.status !== 'open' || !this.wire) {
      this.say('게임 서버와 다시 연결하는 중입니다', true)
      return Promise.resolve({ seq: 0, ok: false, tick: 0 })
    }
    if (type === 'speed') this.head = { ...this.head, speed: Number(body.speed) } // 빠르게 눌러도 이어지도록 표시만 먼저 바꾼다
    const seq = ++this.seq
    this.wire.send(JSON.stringify({ epoch: this.epoch, seq, type, ...body }))
    return new Promise((res) => this.waits.set(seq, (a) => { if (a.ok && DID[type]) sfx(DID[type]); if (!a.ok && a.error) this.say(a.error, true); res(a) }))
  }

  restart(seed?: number) {
    if (this.status === 'open') this.wire?.send(JSON.stringify({ epoch: this.epoch, seq: ++this.seq, type: 'restart', seed: seed ?? 0 }))
  }

  say(text: string, bad = false) { if (bad) sfx('bad'); this.notice = { text, bad, id: ++this.noticeId }; this.changed() }

  setFlip(on: boolean) {
    if (on === this.flip || !this.ready) return false
    this.flip = on
    this.nodes.forEach(turn)
    turnHead(this.head)
    this.config.sources.forEach(turn)
    this.fx = []
    this.changed()
    return true
  }

  private receive(m: any) {
    if (this.flip && m.epoch >= this.epoch) {
      m.nodes?.forEach(turn)
      m.config?.sources.forEach(turn)
      if (m.head) turnHead(m.head)
    }
    if (m.type === 'snapshot') {
      const fresh = m.epoch !== this.epoch
      this.epoch = m.epoch; this.seed = m.seed; this.seq = m.lastSeq; this.config = m.config; this.head = m.head; this.result = m.result
      this.nodes = new Map((m.nodes ?? []).map((n: GNode) => [n.id, n]))
      this.links = new Map((m.links ?? []).map((l: GLink) => [l.id, l]))
      this.lanes.clear(); this.where.clear(); this.strands.clear(); this.queue = []; this.waits.clear()
      for (const [id, link, dep, arr, stage] of m.moving) this.addDot(id, link, dep, arr, stage, true)
      this.rt = m.head.tick
      if (fresh) { this.fx = []; this.logs = []; this.notice = null; this.resets++ }
      this.status = 'open'
      this.ready = true
    } else if (m.epoch !== this.epoch) return
    else if (m.type === 'ack') { this.waits.get(m.ack.seq)?.(m.ack); this.waits.delete(m.ack.seq) }
    else if (m.type === 'frame') {
      if (m.head.phase > this.head.phase) this.grew = m.head.area.w * m.head.area.h > this.head.area.w * this.head.area.h
      if (m.head.reward && !this.head.reward) sfx('reward')
      if (m.head.over && !this.head.over) sfx(m.head.over === 'done' ? 'finish' : 'over')
      this.head = m.head
      for (const n of m.nodes ?? []) {
        const old = this.nodes.get(n.id)
        if (old && n.level > old.level) this.addFx({ kind: 'up', x: n.x, y: n.y, at: performance.now() })
        this.nodes.set(n.id, n)
      }
      for (const l of m.links ?? []) this.links.set(l.id, l)
      for (const id of m.goneNodes ?? []) this.nodes.delete(id)
      for (const id of m.goneLinks ?? []) { this.links.delete(id); this.lanes.delete(`${id}:0`); this.lanes.delete(`${id}:1`) }
      if (m.events) this.queue.push(...m.events)
      if (m.result) this.result = m.result
    }
    this.changed()
  }

  private addDot(id: number, link: number, dep: number, arr: number, stage: number, placed = false) {
    const lane: 0 | 1 = stage >= 2 ? 1 : 0
    const key = `${link}:${lane}`
    const n = (this.strands.get(key) ?? 0) + 1
    this.strands.set(key, n)
    const d: Dot = { id, link, lane, dep, arr, stage, strand: n % Math.min(4, (this.links.get(link)?.level ?? 0) + 1), p: placed ? 1 : 0 }
    const list = this.lanes.get(key)
    if (list) list.push(d)
    else this.lanes.set(key, [d])
    this.where.set(id, d)
  }

  private dropDot(id: number): Dot | undefined {
    const d = this.where.get(id)
    if (!d) return
    this.where.delete(id)
    const list = this.lanes.get(`${d.link}:${d.lane}`)
    const i = list?.indexOf(d) ?? -1
    if (i >= 0) list!.splice(i, 1)
    return d
  }

  private addFx(f: Fx) {
    // 같은 자리의 같은 효과는 잠깐 동안 하나만 보인다. 표시만 줄인다.
    if (f.kind !== 'fail' && this.fx.some((o) => o.kind === f.kind && o.x === f.x && o.y === f.y && f.at - o.at < 330)) return
    this.fx.push(f)
    if (this.fx.length > MAX_FX) this.fx.shift()
  }
  private log(at: number, code: number, text: string) {
    this.logs.push({ id: ++this.logId, at, code, text })
    if (this.logs.length > MAX_LOG) this.logs.shift()
  }

  // 화면 시계를 서버 틱에 맞춰 흘리고, 그 시각까지의 이벤트만 재생한다.
  advance(dt: number) {
    if (!this.ready) return
    const { head, config } = this
    const running = this.status === 'open' && !head.paused && !head.reward && !head.over
    const target = running ? head.tick - 2 * head.speed - 1 : head.tick
    if (running) this.rt += dt * config.tps * head.speed
    const err = target - this.rt
    if (Math.abs(err) > 60) this.rt = target
    else this.rt = Math.max(this.rt, this.rt + err * Math.min(1, dt * 4))
    this.rt = Math.min(this.rt, head.tick)
    const now = performance.now()
    let i = 0
    for (; i < this.queue.length && this.queue[i].at <= this.rt; i++) this.apply(this.queue[i], now)
    if (i) this.queue.splice(0, i)
    while (this.fx.length && now - this.fx[0].at > 1600) this.fx.shift()
  }

  private apply(e: GEvent, now: number) {
    const node = e.n ? this.nodes.get(e.n) : undefined
    switch (e.k) {
      case 'go':
        this.dropDot(e.r!)
        this.addDot(e.r!, e.l!, e.a!, e.b!, e.s ?? 0)
        if (!e.s) { const l = this.links.get(e.l!), n = l && this.nodes.get(l.from); if (n) this.addFx({ kind: 'spawn', x: n.x, y: n.y, at: now }) }
        break
      case 'in': this.dropDot(e.r!); break
      case 'done':
        this.dropDot(e.r!)
        sfx('done')
        if (node) this.addFx({ kind: 'done', x: node.x, y: node.y, at: now })
        this.log(e.at, 200, `GET /order ${((e.a ?? 0) / 1000).toFixed(1)}s`)
        break
      case 'cache': if (node) this.addFx({ kind: 'cache', x: node.x, y: node.y, at: now, text: '캐시' }); break
      case 'fail': {
        const d = this.dropDot(e.r!)
        const at = d ? this.dotPos(d) : null
        const f = FAIL[e.a ?? 0]
        sfx('fail')
        if (at) this.addFx({ kind: 'fail', x: at.x, y: at.y, at: now, text: String(f.code) })
        else if (node) this.addFx({ kind: 'fail', x: node.x, y: node.y, at: now, text: String(f.code) })
        this.log(e.at, f.code, `GET /order ${((e.b ?? 0) / 1000).toFixed(1)}s ${f.name}`)
        break
      }
      case 'fault':
        if (e.t === 'notice' || e.t === 'active') sfx('alert')
        if (e.t === 'ended') { sfx('recover'); this.say('장애가 복구되었습니다') }
        if (e.t === 'canceled') this.say('장애 대상이 사라져 이번 장애는 지나갔습니다')
        break
      case 'source': sfx('source'); this.say('새 클라이언트가 들어왔습니다. 서버로 연결하세요'); break
    }
  }
}
