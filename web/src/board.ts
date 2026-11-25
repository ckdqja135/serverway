import type { Dot, GameClient } from './client'
import { getPref } from './prefs'
import { API, CACHE, DB, SOURCE, nodeName, type GLink, type GNode, type Selection, type Tool, type Area } from './types'

const LIGHT = {
  paper: '#e9ecf7', area: '#f7f8fd', edge: '#d5daee', dot: '#cfd5ea', ink: '#2a2d4f', sub: '#6b7095', shade: 'rgba(52,58,110,0.10)',
  coral: '#ff8a70', coralTop: '#ffe0d6', coralLow: '#e0745b',
  peri: '#6c7cff', periTop: '#e0e4ff', periLow: '#4f5dd6',
  mint: '#2fb79a', mintTop: '#d3f3ea', mintLow: '#1f8f78',
  amber: '#e39a00', red: '#e0404a', tube: '#ffffff', tubeEdge: '#a7b2e8', violet: '#8c5be8', violetTop: '#eadfff', violetLow: '#6a3fc4', ws: '#19a7c4',
  tubeFull: '#ffe9ea', tubeJam: '#fff4dc', tubeMid: '#dfe3f5', arrow: '#aab3e0', slot: '#ffffff', bead: '#c9cfe6', downTop: '#e7e7ee', badTop: '#ffd9db',
}
const DARK: typeof LIGHT = {
  paper: '#14162a', area: '#1b1e37', edge: '#2f3459', dot: '#343a63', ink: '#e9ebff', sub: '#7f86b5', shade: 'rgba(0,0,0,0.32)',
  coral: '#ff8a70', coralTop: '#56342f', coralLow: '#c25f48',
  peri: '#7d8bff', periTop: '#333a72', periLow: '#4f5dd6',
  mint: '#35c9a9', mintTop: '#1f4a45', mintLow: '#1b7f6b',
  amber: '#e39a00', red: '#f0505a', tube: '#2c3157', tubeEdge: '#5b66ad', violet: '#b08cff', violetTop: '#43336e', violetLow: '#6a3fc4', ws: '#2cc0de',
  tubeFull: '#4a2532', tubeJam: '#43371c', tubeMid: '#434a7c', arrow: '#7480c4', slot: '#14162a', bead: '#474e80', downTop: '#3c3e55', badTop: '#4a2532',
}
let C = LIGHT
const MAX_DOTS = 420
const MAX_CELL = 60 // 큰 화면에서 작은 판이 지나치게 커지지 않게 하는 한 칸의 크기

// Hint는 안내 표시다. drag는 끌어서 이을 두 노드, 나머지는 눌러야 할 대상이다.
export interface Hint { drag?: [number, number]; node?: number; link?: number; spot?: { x: number; y: number } }
export interface UIState { selection: Selection; tool: Tool; hint: Hint | null }
// 레벨이 높아도 화면의 가닥 수는 4개까지만 늘린다.
export const strands = (level: number) => Math.min(4, level + 1)

interface Path { pts: { x: number; y: number; nx: number; ny: number }[]; len: number; key: string }

export class Board {
  ui: UIState = { selection: null, tool: 'connect', hint: null }
  onSelect: (s: Selection) => void = () => {}
  onTool: (t: Tool) => void = () => {}
  onZoom: (zoomed: boolean) => void = () => {}
  cover = 0 // 작은 화면에서 위쪽 안내가 가리는 높이
  under = 0 // 작은 화면에서 아래쪽 알림이 가리는 높이
  private ctx: CanvasRenderingContext2D
  private cam = { x: 2, y: 2, w: 17, h: 10 }
  private area: Area = this.cam // 지금 보여 줄 판
  private grow: { from: Area; at: number } | null = null
  private S = 40
  private ox = 0
  private oy = 0
  private paths = new Map<number, Path>()
  private mouse = { x: -1, y: -1, inside: false }
  private drag: { from: number; x: number; y: number; moved: boolean } | null = null
  private raf = 0
  private last = 0
  private resets = -1
  private dashed = 0
  private right = 28
  private top = 104
  private bottom = 104
  private lift = 0
  private view = { z: 1, x: 0, y: 0 } // 두 손가락 확대와 이동
  private fit = { S: 40, cx: 0, cy: 0, max: 1 }
  private zoomed = false
  private touch = false
  private pointers = new Map<number, { x: number; y: number }>()
  private pinch: { d: number; x: number; y: number } | null = null
  private pan: { x: number; y: number; vx: number; vy: number; moved: boolean } | null = null
  private off: (() => void)[] = []

  constructor(private cv: HTMLCanvasElement, private game: GameClient) {
    this.ctx = cv.getContext('2d')!
    game.dotPos = (d) => this.dotAt(d)
    const on = <K extends keyof HTMLElementEventMap>(k: K, fn: (e: HTMLElementEventMap[K]) => void) => {
      cv.addEventListener(k, fn)
      this.off.push(() => cv.removeEventListener(k, fn))
    }
    on('pointerdown', (e) => this.down(e))
    on('pointermove', (e) => this.move(e))
    on('pointerup', (e) => this.up(e))
    on('pointercancel', (e) => { this.pointers.delete(e.pointerId); this.drag = this.pan = null; if (!this.pointers.size) this.pinch = null })
    on('touchend', (e) => { if (e.cancelable) e.preventDefault() }) // 방금 열린 패널의 버튼까지 같은 터치로 눌리지 않게 한다
    on('pointerleave', () => { this.mouse.inside = false })
    on('contextmenu', (e) => { e.preventDefault(); if (this.touch) return; this.drag = null; this.onTool('connect') }) // 길게 누르기로 끌기가 끊기지 않게 한다
    const loop = (t: number) => {
      const dt = Math.min(0.1, (t - this.last) / 1000)
      this.last = t
      this.game.advance(dt)
      if (this.game.ready) this.draw(dt, t)
      this.raf = requestAnimationFrame(loop)
    }
    this.raf = requestAnimationFrame(loop)
  }

  destroy() { cancelAnimationFrame(this.raf); this.off.forEach((f) => f()) }
  cancel() { this.drag = null }
  resetView() { this.view = { z: 1, x: 0, y: 0 } }

  // 좌표

  private sx = (x: number) => this.ox + (x + 0.5) * this.S
  private sy = (y: number) => this.oy + (y + 0.5) * this.S
  private cell(px: number, py: number) { return { x: Math.round((px - this.ox) / this.S - 0.5), y: Math.round((py - this.oy) / this.S - 0.5) } }
  private radius = (n: GNode) => (n.kind === SOURCE ? 0.62 : (n.kind === API ? 0.82 : n.kind === CACHE ? 0.78 : 0.9) * (1 + Math.min(6, n.level) * 0.035))

  private layout(dt: number) {
    const w = this.cv.clientWidth, h = this.cv.clientHeight, r = Math.min(2, window.devicePixelRatio || 1)
    if (this.cv.width !== Math.round(w * r) || this.cv.height !== Math.round(h * r)) { this.cv.width = Math.round(w * r); this.cv.height = Math.round(h * r) }
    this.ctx.setTransform(r, 0, 0, r, 0, 0)
    // styles.css의 작은 화면 기준과 같다.
    const compact = w <= 760 || h <= 500, tall = h > w
    if (this.game.setFlip(tall)) { this.resets = -1; this.resetView() }
    // 보상을 고르는 동안에는 가려져 있으므로, 고른 뒤에 판이 넓어지는 모습을 보여 준다.
    const next = this.game.head.area, fresh = this.resets !== this.game.resets
    if (fresh) { this.area = next; this.grow = null }
    else if (!this.game.head.reward) {
      if (next.w * next.h > this.area.w * this.area.h) this.grow = { from: this.area, at: performance.now() }
      this.area = next
    }
    const a = this.area
    const k = fresh ? 1 : Math.min(1, dt * 1.6)
    if (this.resets !== this.game.resets) this.resetView()
    this.resets = this.game.resets
    this.cam.x += (a.x - this.cam.x) * k; this.cam.y += (a.y - this.cam.y) * k
    this.cam.w += (a.w - this.cam.w) * k; this.cam.h += (a.h - this.cam.h) * k
    // 패널이 열리면 보드를 옆으로 비켜 가려지는 노드가 없게 한다.
    const side = compact ? 8 : 28, left = compact && !tall ? 98 : side, sel = this.ui.selection
    const open = sel && (sel.type === 'node' ? this.game.nodes : this.game.links).has(sel.id)
    const ease = Math.min(1, dt * 9)
    const beside = compact ? !tall : w > 900
    this.right += ((open && beside ? (compact ? 266 : 326) : compact && tall ? 54 : side) - this.right) * ease
    this.top += ((compact ? Math.max(60, this.cover) : 104) - this.top) * ease
    this.bottom += ((compact ? Math.max(tall ? 92 : 10, this.under) : 104) - this.bottom) * ease
    const usable = w - left - this.right, room = h - this.top - this.bottom
    const S = Math.max(4, Math.min(MAX_CELL, usable / this.cam.w, room / this.cam.h))
    this.fit = { S, cx: left + usable / 2, cy: this.top + room / 2, max: Math.max(1, MAX_CELL / S) }
    const v = this.view
    v.z = Math.min(this.fit.max, Math.max(1, v.z))
    this.S = S * v.z
    const mx = Math.max(0, (this.cam.w * this.S - usable) / 2), my = Math.max(0, (this.cam.h * this.S - room) / 2)
    v.x = Math.min(mx, Math.max(-mx, v.x))
    v.y = Math.min(my, Math.max(-my, v.y))
    this.ox = this.fit.cx - (this.cam.x + this.cam.w / 2) * this.S + v.x
    this.oy = this.fit.cy - (this.cam.y + this.cam.h / 2) * this.S + v.y
    // 아래에서 올라온 패널이 고른 대상을 가리면 그만큼 보드를 올린다.
    const at = compact && tall && open ? this.center(sel!) : null
    this.lift += ((at ? Math.max(0, this.sy(at.y) - (h * 0.54 - 84)) : 0) - this.lift) * ease
    this.oy -= this.lift
    if (v.z > 1.03 !== this.zoomed) { this.zoomed = !this.zoomed; this.onZoom(this.zoomed) }
    this.dashed += dt * 26
  }

  private center(s: NonNullable<Selection>) {
    if (s.type === 'node') return this.game.nodes.get(s.id) ?? null
    const l = this.game.links.get(s.id), p = l && this.path(l)
    return p ? this.at(p, 0.5, 0) : null
  }

  private path(l: GLink): Path | null {
    const a = this.game.nodes.get(l.from), b = this.game.nodes.get(l.to)
    if (!a || !b) return this.paths.get(l.id) ?? null
    const key = `${a.x},${a.y},${b.x},${b.y}`
    const old = this.paths.get(l.id)
    if (old?.key === key) return old
    const p = curve(a.x, a.y, b.x, b.y, this.radius(a) * 0.9, this.radius(b) * 0.95, key)
    this.paths.set(l.id, p)
    return p
  }

  private at(p: Path, t: number, off: number) {
    const f = Math.min(0.9999, Math.max(0, t)) * (p.pts.length - 1)
    const i = Math.floor(f), k = f - i, a = p.pts[i], b = p.pts[i + 1]
    return { x: a.x + (b.x - a.x) * k + a.nx * off, y: a.y + (b.y - a.y) * k + a.ny * off }
  }

  private dotAt(d: Dot) {
    const l = this.game.links.get(d.link)
    const p = l && this.path(l)
    return p ? this.at(p, d.lane ? 1 - d.p : d.p, this.offset(l!, d.lane, d.strand)) : null
  }

  private offset = (l: GLink, lane: number, strand: number) => (lane ? -1 : 1) * (0.1 + (strand % strands(l.level)) * 0.13)

  // 입력

  private pos(e: PointerEvent) { const r = this.cv.getBoundingClientRect(); return { x: e.clientX - r.left, y: e.clientY - r.top } }
  private nodeAt(px: number, py: number, extra = 0.18) {
    let best: GNode | null = null, bd = Infinity
    for (const n of this.game.nodes.values()) {
      const d = Math.hypot(px - this.sx(n.x), py - this.sy(n.y))
      if (d < Math.max((this.radius(n) + extra) * this.S, this.touch ? 24 : 0) && d < bd) { best = n; bd = d }
    }
    return best
  }
  private linkAt(px: number, py: number) {
    let best: GLink | null = null, bd = this.touch ? 22 : 13
    for (const l of this.game.links.values()) {
      const p = this.path(l)
      for (const q of p?.pts ?? []) {
        const d = Math.hypot(px - this.sx(q.x), py - this.sy(q.y))
        if (d < bd) { best = l; bd = d }
      }
    }
    return best
  }

  private down(e: PointerEvent) {
    if (e.button !== 0 || !this.game.ready || this.game.head.over) return
    const { x, y } = this.pos(e)
    this.cv.setPointerCapture(e.pointerId)
    this.touch = e.pointerType !== 'mouse'
    this.pointers.set(e.pointerId, { x, y })
    if (this.pointers.size > 1) {
      this.drag = this.pan = null
      this.mouse.inside = false
      this.pinch = this.spread()
      return
    }
    this.mouse = { x, y, inside: true }
    if (this.ui.tool !== 'connect') return
    const n = this.nodeAt(x, y)
    if (n && n.kind < DB && !n.draining) this.drag = { from: n.id, x, y, moved: false }
    else if (this.zoomed) this.pan = { x, y, vx: this.view.x, vy: this.view.y, moved: false }
  }

  private spread() {
    const [a, b] = [...this.pointers.values()]
    return { d: Math.max(1, Math.hypot(a.x - b.x, a.y - b.y)), x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 }
  }

  // 두 손가락 가운데에 있던 보드 위치가 손가락을 따라오게 한다.
  private zoom() {
    const p = this.pinch!, q = this.spread(), f = this.fit, c = this.cam
    const z = Math.min(f.max, Math.max(1, (this.view.z * q.d) / p.d)), S = f.S * z
    const bx = (p.x - this.ox) / this.S, by = (p.y - this.oy) / this.S
    this.view = { z, x: q.x - bx * S - f.cx + (c.x + c.w / 2) * S, y: q.y - by * S - f.cy + (c.y + c.h / 2) * S }
    this.S = S; this.ox = q.x - bx * S; this.oy = q.y - by * S
    this.pinch = q
  }

  private move(e: PointerEvent) {
    const { x, y } = this.pos(e)
    if (this.pointers.has(e.pointerId)) this.pointers.set(e.pointerId, { x, y })
    if (this.pinch) { if (this.pointers.size > 1) this.zoom(); return }
    this.mouse = { x, y, inside: true }
    if (this.pan) {
      const dx = x - this.pan.x, dy = y - this.pan.y
      if (Math.hypot(dx, dy) > 7) this.pan.moved = true
      if (this.pan.moved) { this.view.x = this.pan.vx + dx; this.view.y = this.pan.vy + dy }
    }
    if (this.drag && Math.hypot(x - this.drag.x, y - this.drag.y) > 7) this.drag.moved = true
    this.cv.style.cursor = this.ui.tool !== 'connect' ? 'copy' : this.drag?.moved ? 'crosshair' : this.nodeAt(x, y) ? 'grab' : this.linkAt(x, y) ? 'pointer' : 'default'
  }

  private up(e: PointerEvent) {
    this.pointers.delete(e.pointerId)
    if (this.pinch) { if (!this.pointers.size) this.pinch = null; return }
    if (e.button !== 0 || !this.game.ready) return
    const { x, y } = this.pos(e)
    const d = this.drag, panned = this.pan?.moved
    this.drag = this.pan = null
    if (this.touch) this.mouse.inside = false
    if (panned) return
    if (this.ui.tool !== 'connect') {
      const c = this.cell(x, y)
      const why = this.buildError(c.x, c.y)
      if (why) return this.game.say(why, true)
      this.game.send('build', { x: this.game.flip ? c.y : c.x, y: this.game.flip ? c.x : c.y, kind: this.ui.tool }).then((a) => { if (a.ok && a.id) { this.onSelect({ type: 'node', id: a.id }); this.onTool('connect') } })
      return
    }
    if (d?.moved) {
      const to = this.nodeAt(x, y, 0.45)
      if (!to || to.id === d.from) return
      const why = this.linkError(this.game.nodes.get(d.from), to)
      if (why) return this.game.say(why, true)
      this.game.send('link', { from: d.from, to: to.id })
      return
    }
    const n = this.nodeAt(x, y)
    const l = n ? null : this.linkAt(x, y)
    this.onSelect(n ? { type: 'node', id: n.id } : l ? { type: 'link', id: l.id } : null)
  }

  // 안내용 검사. 최종 판단은 서버가 한다.

  linkCost(a: GNode, b: GNode) { const c = this.game.config; return c.linkBase + c.linkPerCell * Math.ceil(Math.hypot(a.x - b.x, a.y - b.y) - 1e-9) }

  linkError(a: GNode | undefined, b: GNode): string | null {
    if (!a) return '연결할 대상이 없습니다'
    if (a.kind === SOURCE && b.kind >= DB) return '요청은 서버 처리를 먼저 거쳐야 합니다'
    if (b.kind === SOURCE) return '응답은 같은 링크로 돌아오므로 따로 연결하지 않아도 됩니다'
    if (a.kind === b.kind) return '같은 종류끼리는 연결할 수 없습니다'
    if (a.draining || b.draining) return '철거 중이라 연결할 수 없습니다'
    for (const l of this.game.links.values()) if (l.from === a.id && l.to === b.id) return l.draining ? '철거가 끝난 뒤 다시 연결할 수 있습니다' : '이미 연결되어 있습니다'
    const cost = this.linkCost(a, b)
    if (cost > this.game.head.budget) return `예산이 부족합니다 (필요 ${cost})`
    return null
  }

  buildError(x: number, y: number): string | null {
    const { config: c, head } = this.game, a = head.area
    if (x < a.x + 1 || y < a.y + 1 || x > a.x + a.w - 2 || y > a.y + a.h - 2) return '아직 쓸 수 없는 자리입니다'
    for (const n of this.game.nodes.values()) if (Math.hypot(n.x - x, n.y - y) < c.minNodeGap) return '다른 노드와 너무 가깝습니다'
    for (const s of c.sources) if (Math.hypot(s.x - x, s.y - y) < c.minNodeGap) return '새 클라이언트가 들어올 자리입니다'
    for (const s of head.upcoming) if (Math.hypot(s.x - x, s.y - y) < c.minNodeGap) return '새 클라이언트가 들어올 자리입니다'
    if (head.servers >= c.maxServers) return `모두 ${c.maxServers}대까지 놓을 수 있습니다`
    const cost = this.buildCost()
    if (cost > head.budget) return `예산이 부족합니다 (필요 ${cost})`
    return null
  }

  private buildCost() { const h = this.game.head; return this.ui.tool === 'db' ? h.dbCost : this.ui.tool === 'redis' ? h.redisCost : h.apiCost }

  // 그리기

  private draw(dt: number, now: number) {
    this.layout(dt)
    C = getPref('dark') ? DARK : LIGHT
    const g = this.ctx
    g.fillStyle = C.paper
    g.fillRect(0, 0, this.cv.clientWidth, this.cv.clientHeight)
    this.ground()
    for (const id of this.paths.keys()) if (!this.game.links.has(id)) this.paths.delete(id)
    const links = [...this.game.links.values()]
    const drag = this.drag?.moved ? this.game.nodes.get(this.drag.from) : undefined
    for (const l of links) this.link(l)
    this.dots(dt)
    for (const u of this.game.head.upcoming) this.ghostSource(u, now)
    for (const n of this.game.nodes.values()) this.node(n, now, drag)
    for (const l of links) this.linkBadge(l)
    for (const n of this.game.nodes.values()) this.badge(n)
    this.faults(now)
    this.effects(now)
    if (this.ui.hint) this.hint(this.ui.hint, now)
    if (drag) this.ghostLink(drag)
    if (this.ui.tool !== 'connect' && this.mouse.inside) this.ghostServer()
  }

  private ground() {
    const g = this.ctx, c = this.cam, S = this.S
    const x = this.ox + c.x * S, y = this.oy + c.y * S
    g.fillStyle = C.area
    g.strokeStyle = C.edge
    g.lineWidth = 1.5
    g.beginPath(); g.roundRect(x, y, c.w * S, c.h * S, S * 0.7); g.fill(); g.stroke()
    const t = this.grow ? (performance.now() - this.grow.at) / 2600 : 1
    if (t < 1) {
      const o = this.grow!.from
      g.globalAlpha = 0.32 * (1 - t)
      g.fillStyle = C.peri
      g.beginPath(); g.roundRect(x, y, c.w * S, c.h * S, S * 0.7); g.roundRect(this.ox + o.x * S, this.oy + o.y * S, o.w * S, o.h * S, S * 0.7); g.fill('evenodd')
      g.globalAlpha = 1
    } else this.grow = null
    g.fillStyle = C.dot
    // 보드가 넓어지면 점이 천 개를 넘으므로 한 번에 그린다.
    const r = Math.max(1.6, S * 0.065)
    g.beginPath()
    for (let i = Math.ceil(c.x + 0.5); i < c.x + c.w - 0.5; i++) for (let j = Math.ceil(c.y + 0.5); j < c.y + c.h - 0.5; j++) g.rect(this.sx(i) - r / 2, this.sy(j) - r / 2, r, r)
    g.fill()
  }

  private trace(p: Path, off = 0, from = 0, to = 1) {
    const g = this.ctx, n = p.pts.length - 1
    g.beginPath()
    for (let i = Math.floor(from * n); i <= Math.ceil(to * n); i++) {
      const q = p.pts[Math.min(n, Math.max(0, i))]
      const X = this.sx(q.x + q.nx * off), Y = this.sy(q.y + q.ny * off)
      if (i === Math.floor(from * n)) g.moveTo(X, Y)
      else g.lineTo(X, Y)
    }
  }

  private link(l: GLink) {
    const p = this.path(l)
    if (!p) return
    const g = this.ctx, S = this.S
    const width = (0.08 + strands(l.level) * 0.26) * S
    const sel = this.ui.selection?.type === 'link' && this.ui.selection.id === l.id
    const edge = l.state === 'blocked' ? C.red : l.state === 'full' ? C.red : l.state === 'jam' ? C.amber : sel ? C.peri : l.ws ? C.ws : C.tubeEdge
    g.lineCap = 'butt'
    g.lineJoin = 'round'
    g.globalAlpha = l.draining ? 0.45 : 1
    this.trace(p); g.strokeStyle = C.area; g.lineWidth = width + 9; g.stroke() // 교차해도 이어지지 않음을 보이는 틈
    this.trace(p); g.strokeStyle = edge; g.lineWidth = width + (sel ? 6 : 3.5); g.stroke()
    this.trace(p); g.strokeStyle = l.state === 'full' ? C.tubeFull : l.state === 'jam' ? C.tubeJam : C.tube; g.lineWidth = width; g.stroke()
    this.trace(p); g.strokeStyle = l.ws ? C.ws : C.tubeMid; g.lineWidth = l.ws ? 2 : 1; g.setLineDash(l.ws ? [] : [3, 5]); g.stroke(); g.setLineDash([])
    // 나가는 방향 표시
    g.strokeStyle = l.ws ? C.ws : C.arrow
    g.lineWidth = 1.6
    for (const t of [0.28, 0.55, 0.82]) {
      const off = 0.035 + strands(l.level) * 0.065
      const a = this.at(p, t, off), b = this.at(p, t + 0.015, off)
      const ang = Math.atan2(b.y - a.y, b.x - a.x), X = this.sx(a.x), Y = this.sy(a.y), k = S * 0.085
      g.beginPath()
      g.moveTo(X - Math.cos(ang - 0.7) * k, Y - Math.sin(ang - 0.7) * k); g.lineTo(X, Y); g.lineTo(X - Math.cos(ang + 0.7) * k, Y - Math.sin(ang + 0.7) * k)
      g.stroke()
    }
    g.globalAlpha = 1
  }

  private dots(dt: number) {
    const g = this.ctx, S = this.S, rt = this.game.rt
    let total = 0
    for (const list of this.game.lanes.values()) total += list.length
    const skip = total > MAX_DOTS ? Math.ceil(total / MAX_DOTS) : 1 // 그리는 수만 줄인다. 게임 계산과 무관하다.
    for (const [key, list] of this.game.lanes) {
      const l = this.game.links.get(Number(key.split(':')[0]))
      const p = l && this.path(l)
      if (!l || !p) continue
      const gap = 0.5 / Math.max(1, p.len)
      const front: number[] = []
      list.forEach((d) => {
        if (rt < d.dep) return
        const free = Math.min(1, (rt - d.dep) / Math.max(1, d.arr - d.dep))
        const lead = front[d.strand]
        const want = Math.max(0, lead === undefined ? free : Math.min(free, lead - gap))
        const step = (dt * 20 * this.game.head.speed * 3) / Math.max(1, d.arr - d.dep)
        d.p = want < d.p ? want : Math.min(want, d.p + step)
        front[d.strand] = d.p
        if (d.id % skip) return
        const q = this.at(p, d.lane ? 1 - d.p : d.p, this.offset(l, d.lane, d.strand))
        const X = this.sx(q.x), Y = this.sy(q.y), r = S * 0.105
        const col = d.stage === 0 ? C.coral : d.stage === 1 ? (this.game.nodes.get(l.to)?.kind === CACHE ? C.violet : C.peri) : C.mint
        g.globalAlpha = 0.28
        g.fillStyle = col
        g.beginPath(); g.arc(X, Y, r * 1.9, 0, 7); g.fill()
        g.globalAlpha = 1
        g.beginPath()
        if (d.lane) g.roundRect(X - r * 0.85, Y - r * 0.85, r * 1.7, r * 1.7, r * 0.5) // 응답은 네모, 요청은 동그라미
        else g.arc(X, Y, r, 0, 7)
        g.fill()
        g.fillStyle = '#fff'
        g.beginPath(); g.arc(X, Y, r * 0.38, 0, 7); g.fill()
      })
    }
  }

  private plate(shape: (dy: number) => void, top: string, low: string, rim: string) {
    const g = this.ctx, S = this.S
    g.fillStyle = C.shade
    shape(0.26 * S); g.fill()
    g.fillStyle = low
    shape(0.13 * S); g.fill()
    g.fillStyle = top
    shape(0); g.fill()
    g.strokeStyle = rim
    g.lineWidth = Math.max(1.5, S * 0.05)
    g.stroke()
  }

  private hex(x: number, y: number, r: number) {
    const g = this.ctx
    g.beginPath()
    for (let i = 0; i < 6; i++) { const a = Math.PI / 6 + (i * Math.PI) / 3; g.lineTo(x + Math.cos(a) * r, y + Math.sin(a) * r) }
    g.closePath()
  }

  private node(n: GNode, now: number, drag?: GNode) {
    const g = this.ctx, S = this.S, x = this.sx(n.x), y = this.sy(n.y)
    const r = this.radius(n) * S
    const sel = this.ui.selection?.type === 'node' && this.ui.selection.id === n.id
    if (drag && drag.id !== n.id) {
      const ok = this.linkError(drag, n) === null
      if (ok) { g.strokeStyle = C.peri; g.lineWidth = 3; g.setLineDash([7, 6]); g.lineDashOffset = -this.dashed; g.beginPath(); g.arc(x, y, r + S * 0.32, 0, 7); g.stroke(); g.setLineDash([]) }
      else g.globalAlpha = 0.4
    }
    if (n.draining) g.globalAlpha = 0.5
    if (sel) { g.fillStyle = 'rgba(108,124,255,0.16)'; g.beginPath(); g.arc(x, y, r + S * 0.36, 0, 7); g.fill() }
    const working = n.jobs.some((j) => !j.d) && n.speed > 0
    const bob = working && !this.game.head.paused ? Math.sin(now / 130 + n.id) * S * 0.012 : 0
    if (n.kind === SOURCE) this.source(x, y, r, now)
    else if (n.kind === API) this.api(n, x, y + bob, r)
    else if (n.kind === CACHE) this.redis(n, x, y + bob, r)
    else this.db(n, x, y + bob, r)
    this.beads(n, x, y, r)
    g.globalAlpha = 1
    g.textAlign = 'center'
    g.textBaseline = 'middle'
    g.font = `600 ${Math.max(10, S * 0.27)}px "Gothic A1"`
    g.fillStyle = C.ink
    const name = nodeName(n)
    g.fillText(n.kind === SOURCE ? name : `${name}  Lv${n.level + 1}`, x, y + r + S * 0.62)
    if (n.store) this.storage(n.store.data / n.store.cap, x, y + r + S * 0.92)
  }

  private source(x: number, y: number, r: number, now: number) {
    const g = this.ctx, S = this.S
    this.plate((dy) => { g.beginPath(); g.arc(x, y + dy, r, 0, 7) }, C.coralTop, C.coralLow, C.coral)
    g.strokeStyle = C.coral
    g.lineWidth = Math.max(1.5, S * 0.055)
    const wave = (now / 900) % 1
    for (let i = 0; i < 2; i++) {
      const k = r * (0.36 + i * 0.28)
      for (const a of [-Math.PI / 2, Math.PI / 6, (Math.PI * 5) / 6]) { g.beginPath(); g.arc(x, y, k, a - 0.45, a + 0.45); g.stroke() }
    }
    g.fillStyle = C.coral
    g.beginPath(); g.arc(x, y, r * (0.17 + 0.03 * Math.sin(wave * 6.28)), 0, 7); g.fill()
  }

  private api(n: GNode, x: number, y: number, r: number) {
    const down = n.state === 'down'
    this.plate((dy) => this.hex(x, y + dy, r), down ? C.downTop : C.periTop, down ? '#9a9cb0' : C.periLow, down ? '#8a8ca3' : C.peri)
    if (down) this.hatch(() => this.hex(x, y, r))
    if (n.slots > 7) return this.gauge(x, y, r * 0.62, n, C.peri)
    const k = n.slots, rr = k <= 2 ? r * 0.3 : k <= 4 ? r * 0.25 : r * 0.2
    for (let i = 0; i < k; i++) {
      const a = k === 1 ? 0 : (i * 2 * Math.PI) / k - Math.PI / 2 + (k === 2 ? Math.PI / 2 : 0)
      const d = k === 1 ? 0 : k === 2 ? r * 0.36 : k <= 4 ? r * 0.42 : r * 0.5
      this.slot(x + Math.cos(a) * d, y + Math.sin(a) * d, rr, n.jobs[i], n, C.peri)
    }
  }

  // 슬롯이 많아지면 하나씩 그리지 않고 사용량 고리로 모아 보여 준다.
  private gauge(x: number, y: number, r: number, n: GNode, col: string) {
    const g = this.ctx, TAU = Math.PI * 2
    const busy = n.jobs.filter((j) => !j.d).length / n.slots, held = n.jobs.filter((j) => j.d).length / n.slots
    g.lineCap = 'butt'
    g.lineWidth = r * 0.34
    g.strokeStyle = C.slot
    g.beginPath(); g.arc(x, y, r, 0, TAU); g.stroke()
    g.strokeStyle = n.speed === 0 ? '#8a8ca3' : col
    g.beginPath(); g.arc(x, y, r, -TAU / 4, -TAU / 4 + busy * TAU); g.stroke()
    if (held > 0) { g.strokeStyle = C.amber; g.beginPath(); g.arc(x, y, r, -TAU / 4 + busy * TAU, -TAU / 4 + (busy + held) * TAU); g.stroke() }
    g.fillStyle = C.ink
    g.textAlign = 'center'; g.textBaseline = 'middle'
    g.font = `800 ${r * 0.72}px "Gothic A1"`
    g.fillText(String(n.slots), x, y + 1)
  }

  private slot(x: number, y: number, r: number, j: GNode['jobs'][number] | undefined, n: GNode, col: string) {
    const g = this.ctx
    g.fillStyle = C.slot
    g.strokeStyle = col
    g.lineWidth = 1.4
    g.beginPath(); g.arc(x, y, r, 0, 7); g.fill(); g.globalAlpha *= 0.5; g.stroke(); g.globalAlpha = n.draining ? 0.5 : 1
    if (!j) return
    if (j.d) { g.fillStyle = C.amber; g.beginPath(); g.arc(x, y, r * 0.62, 0, 7); g.fill(); return } // 처리했지만 보낼 길이 막힘
    const p = j.e > j.s ? Math.min(1, Math.max(0.04, (this.game.rt - j.s) / (j.e - j.s))) : 0.5
    g.fillStyle = n.speed === 0 ? '#8a8ca3' : col
    g.beginPath(); g.moveTo(x, y); g.arc(x, y, r * 0.8, -Math.PI / 2, -Math.PI / 2 + p * 2 * Math.PI); g.closePath(); g.fill()
  }

  // 저장 공간이 찬 정도
  private storage(full: number, x: number, y: number) {
    const g = this.ctx, w = this.S * 1.7, h = Math.max(4, this.S * 0.13)
    g.fillStyle = C.bead
    g.beginPath(); g.roundRect(x - w / 2, y, w, h, h / 2); g.fill()
    g.fillStyle = full >= 1 ? C.red : full * 100 >= this.game.config.storageWarn ? C.amber : C.mint
    g.beginPath(); g.roundRect(x - w / 2, y, Math.max(h, w * Math.min(1, full)), h, h / 2); g.fill()
  }

  private redis(n: GNode, x: number, y: number, r: number) {
    const g = this.ctx
    this.plate((dy) => { g.beginPath(); g.roundRect(x - r * 0.92, y + dy - r * 0.92, r * 1.84, r * 1.84, r * 0.42) }, C.violetTop, C.violetLow, C.violet)
    // 번개
    g.globalAlpha *= 0.28
    g.fillStyle = C.violet
    g.beginPath()
    for (const [a, b] of [[0.12, -0.72], [-0.46, 0.12], [-0.04, 0.12], [-0.14, 0.72], [0.46, -0.14], [0.04, -0.14]]) g.lineTo(x + a * r, y + b * r)
    g.closePath(); g.fill()
    g.globalAlpha = n.draining ? 0.5 : 1
    if (n.slots > 4) return this.gauge(x, y, r * 0.56, n, C.violet)
    for (let i = 0; i < n.slots; i++) this.slot(x + (i % 2 ? 0.4 : -0.4) * r, y + (i < 2 ? -0.4 : 0.4) * r, r * 0.25, n.jobs[i], n, C.violet)
  }

  private db(n: GNode, x: number, y: number, r: number) {
    const g = this.ctx
    const disc = (dy: number) => { g.beginPath(); g.ellipse(x, y + dy, r, r * 0.62, 0, 0, 7) }
    g.fillStyle = C.shade
    disc(r * 0.72); g.fill()
    g.lineWidth = Math.max(1.5, this.S * 0.05)
    for (const dy of [0.42, 0.14, -0.14]) {
      g.fillStyle = C.mintLow; disc((dy + 0.12) * r); g.fill()
      g.fillStyle = C.mintTop; disc(dy * r); g.fill()
      g.strokeStyle = C.mint; g.stroke()
    }
    if (n.slots > 7) return this.gauge(x, y - r * 0.14, r * 0.4, n, C.mint)
    const k = n.slots, rr = Math.min(r * 0.2, (r * 1.5) / k / 2.2)
    for (let i = 0; i < k; i++) this.slot(x + (i - (k - 1) / 2) * rr * 2.25, y - r * 0.14, rr, n.jobs[i], n, C.mint)
  }

  // 대기열 구슬. 상한만큼 자리가 있고, 찰수록 색이 바뀐다.
  private beads(n: GNode, x: number, y: number, r: number) {
    const g = this.ctx, S = this.S
    const show = Math.min(n.limit, 24)
    const R = r + S * 0.24, br = S * 0.075
    const span = Math.min(Math.PI * 1.3, show * 0.27)
    const full = n.queue / Math.max(1, n.limit)
    const col = full >= 0.8 ? C.red : full >= 0.4 ? C.amber : n.kind === SOURCE ? C.coral : C.peri
    const filled = Math.round(full * show)
    for (let i = 0; i < show; i++) {
      const a = -Math.PI / 2 - span / 2 + (show === 1 ? 0 : (i * span) / (show - 1))
      const X = x + Math.cos(a) * R, Y = y + Math.sin(a) * R * (n.kind === DB ? 0.9 : 1)
      g.beginPath(); g.arc(X, Y, i < filled ? br : br * 0.55, 0, 7)
      g.fillStyle = i < filled ? col : C.bead
      g.fill()
    }
    if (n.out > 0) { g.fillStyle = C.mint; g.font = `600 ${S * 0.22}px "Gothic A1"`; g.textAlign = 'left'; g.fillText(`응답 대기 ${n.out}`, x + r * 0.9, y + r * 0.7) }
  }

  private hatch(shape: () => void) {
    const g = this.ctx
    g.save()
    shape(); g.clip()
    g.strokeStyle = 'rgba(90,92,120,0.35)'
    g.lineWidth = 3
    const w = this.cv.clientWidth + this.cv.clientHeight
    for (let i = 0; i < w; i += 11) { g.beginPath(); g.moveTo(i, 0); g.lineTo(i - this.cv.clientHeight, this.cv.clientHeight); g.stroke() }
    g.restore()
  }

  private pill(text: string, x: number, y: number, bg: string, icon?: 'warn' | 'stop' | 'clock') {
    const g = this.ctx, f = Math.max(11, this.S * 0.26)
    g.font = `800 ${f}px "Gothic A1"`
    g.textAlign = 'left'
    g.textBaseline = 'middle'
    const w = g.measureText(text).width + (icon ? f * 1.25 : 0) + f, h = f * 1.65
    g.fillStyle = bg
    g.beginPath(); g.roundRect(x - w / 2, y - h / 2, w, h, h / 2); g.fill()
    let tx = x - w / 2 + f / 2
    if (icon) {
      const cx = tx + f * 0.45, k = f * 0.45
      g.strokeStyle = '#fff'; g.lineWidth = 1.8; g.lineJoin = 'round'
      g.beginPath()
      if (icon === 'warn') { g.moveTo(cx, y - k); g.lineTo(cx + k, y + k * 0.8); g.lineTo(cx - k, y + k * 0.8); g.closePath(); g.stroke(); g.beginPath(); g.moveTo(cx, y - k * 0.25); g.lineTo(cx, y + k * 0.35) }
      else if (icon === 'stop') { g.moveTo(cx - k * 0.7, y - k * 0.7); g.lineTo(cx + k * 0.7, y + k * 0.7); g.moveTo(cx + k * 0.7, y - k * 0.7); g.lineTo(cx - k * 0.7, y + k * 0.7) }
      else { g.arc(cx, y, k * 0.85, 0, 7); g.moveTo(cx, y - k * 0.5); g.lineTo(cx, y); g.lineTo(cx + k * 0.4, y + k * 0.2) }
      g.stroke()
      tx += f * 1.25
    }
    g.fillStyle = '#fff'
    g.fillText(text, tx, y + 0.5)
  }

  private badge(n: GNode) {
    const x = this.sx(n.x), y = this.sy(n.y) - (this.radius(n) + 0.72) * this.S
    const fault = this.game.head.faults.some((f) => f.node === n.id && f.state === 'active')
    if (fault) return
    if (n.kind === SOURCE) {
      if (!n.routed) this.pill('연결 필요', x, y, C.coralLow, 'warn')
      else if (n.state === 'full') this.pill('넘치기 직전', x, y, C.red, 'warn')
      return
    }
    // 앞이 막혀서 찬 것과 스스로 한계인 것을 구분한다.
    const stuck = n.stuck > 0 && n.stuck >= n.slots
    if (n.state === 'full' && !stuck) this.pill('처리 한계', x, y, C.red, 'warn')
    else if (stuck) this.pill('앞이 막힘', x, y, C.amber)
    else if (n.store && n.store.data >= n.store.cap) this.pill('저장 공간 가득', x, y, C.red, 'warn')
    else if (n.store && n.store.data * 100 >= n.store.cap * this.game.config.storageWarn) this.pill(`저장 공간 ${Math.floor((n.store.data / n.store.cap) * 100)}%`, x, y, C.amber, 'warn')
  }

  private linkBadge(l: GLink) {
    const p = this.path(l)
    if (!p) return
    const m = this.at(p, 0.5, 0), x = this.sx(m.x), y = this.sy(m.y)
    if (l.state === 'blocked') {
      const g = this.ctx, k = this.S * 0.34
      for (const t of [0.35, 0.65]) {
        const q = this.at(p, t, 0)
        g.fillStyle = C.red
        g.beginPath(); g.roundRect(this.sx(q.x) - k / 2, this.sy(q.y) - k, k, k * 2, 4); g.fill()
        g.strokeStyle = '#fff'; g.lineWidth = 2.5
        g.beginPath(); g.moveTo(this.sx(q.x) - k * 0.3, this.sy(q.y) - k * 0.3); g.lineTo(this.sx(q.x) + k * 0.3, this.sy(q.y) + k * 0.3)
        g.moveTo(this.sx(q.x) + k * 0.3, this.sy(q.y) - k * 0.3); g.lineTo(this.sx(q.x) - k * 0.3, this.sy(q.y) + k * 0.3); g.stroke()
      }
    } else if (l.state === 'full') this.pill('전송 한계', x, y - this.S * 0.62, C.red, 'warn')
    else if (l.draining) this.pill(`철거 중 ${l.pending}`, x, y - this.S * 0.6, C.sub)
  }

  private faults(now: number) {
    const g = this.ctx, S = this.S
    for (const f of this.game.head.faults) {
      const n = f.node ? this.game.nodes.get(f.node) : undefined
      const l = f.link ? this.game.links.get(f.link) : undefined
      const p = l && this.path(l)
      const at = n ? { x: n.x, y: n.y } : p ? this.at(p, 0.5, 0) : null
      if (!at) continue
      const x = this.sx(at.x), y = this.sy(at.y), r = n ? (this.radius(n) + 0.42) * S : S * 0.75
      const active = f.state === 'active'
      const left = Math.max(0, Math.ceil(((active ? f.endAt : f.startAt) - this.game.rt) / this.game.config.tps))
      g.strokeStyle = active ? C.red : C.amber
      g.lineWidth = 3.5
      g.setLineDash(active ? [] : [10, 7])
      g.lineDashOffset = -this.dashed
      g.globalAlpha = active ? 1 : 0.65 + 0.35 * Math.sin(now / 220)
      g.beginPath(); g.arc(x, y, r, 0, 7); g.stroke()
      g.setLineDash([])
      g.globalAlpha = 1
      const name = f.type === 'api_down' ? '멈춤' : f.type === 'api_slow' ? '느려짐' : '단선'
      this.pill(active ? `${name} ${left}초` : `${name} 예고 ${left}초`, x, y - r - S * 0.34, active ? C.red : C.amber, active ? 'stop' : 'clock')
    }
  }

  private effects(now: number) {
    const g = this.ctx, S = this.S
    for (const e of this.game.fx) {
      const t = (now - e.at) / (e.kind === 'fail' ? 1400 : e.kind === 'gain' ? 2400 : 800)
      if (t < 0 || t > 1) continue
      const x = this.sx(e.x), y = this.sy(e.y)
      g.globalAlpha = e.kind === 'gain' ? Math.min(1, (1 - t) * 3) : 1 - t
      g.textAlign = 'center'
      g.textBaseline = 'middle'
      if (e.kind === 'fail') {
        const yy = y - t * S * 0.8, k = S * 0.13
        g.strokeStyle = C.red; g.lineWidth = 3; g.lineCap = 'round'
        g.beginPath(); g.moveTo(x - k, yy - k); g.lineTo(x + k, yy + k); g.moveTo(x + k, yy - k); g.lineTo(x - k, yy + k); g.stroke()
        g.fillStyle = C.red; g.font = `800 ${S * 0.28}px "Gothic A1"`
        g.fillText(e.text ?? '', x, yy - S * 0.32)
      } else if (e.kind === 'gain') {
        this.pill(e.text ?? '', x, y - Math.min(1, t * 4) * S * 0.35, C.mintLow)
      } else if (e.kind === 'cache') {
        g.fillStyle = C.violet; g.font = `800 ${S * 0.26}px "Gothic A1"`
        g.fillText('캐시', x + S * 0.5, y - S * 0.3 - t * S * 0.5)
      } else {
        g.strokeStyle = e.kind === 'done' ? C.mint : e.kind === 'up' ? C.peri : C.coral
        g.lineWidth = e.kind === 'spawn' ? 1.6 : 3.5 * (1 - t) + 1
        g.beginPath(); g.arc(x, y, S * (e.kind === 'up' ? 0.8 + t * 1.2 : 0.6 + t * 0.55), 0, 7); g.stroke()
      }
      g.globalAlpha = 1
    }
  }

  private ghostSource(u: { x: number; y: number; at: number }, now: number) {
    const g = this.ctx, S = this.S, x = this.sx(u.x), y = this.sy(u.y)
    g.strokeStyle = C.coral; g.lineWidth = 2; g.setLineDash([5, 5])
    g.globalAlpha = 0.6 + 0.3 * Math.sin(now / 250)
    g.beginPath(); g.arc(x, y, S * 0.62, 0, 7); g.stroke()
    g.setLineDash([]); g.globalAlpha = 1
    this.pill(`새 클라이언트 ${Math.max(0, Math.ceil((u.at - this.game.rt) / this.game.config.tps))}초`, x, y - S, C.coralLow, 'clock')
  }

  private ghostLink(from: GNode) {
    const g = this.ctx, S = this.S
    const over = this.nodeAt(this.mouse.x, this.mouse.y, 0.45)
    const to = over && over.id !== from.id ? over : null
    const why = to ? this.linkError(from, to) : null
    const ex = to ? this.sx(to.x) : this.mouse.x, ey = to ? this.sy(to.y) : this.mouse.y
    g.strokeStyle = why ? C.red : C.peri
    g.lineWidth = S * 0.16
    g.lineCap = 'round'
    g.globalAlpha = 0.55
    g.setLineDash([S * 0.22, S * 0.24]); g.lineDashOffset = -this.dashed
    g.beginPath(); g.moveTo(this.sx(from.x), this.sy(from.y)); g.lineTo(ex, ey); g.stroke()
    g.setLineDash([]); g.globalAlpha = 1
    if (to) this.pill(why ?? `연결 비용 ${this.linkCost(from, to)}`, ex, ey + (this.touch ? -1.9 : 1.35) * S, why ? C.red : C.periLow, why ? 'stop' : undefined)
  }

  private ghostServer() {
    const g = this.ctx, S = this.S, c = this.cell(this.mouse.x, this.mouse.y), tool = this.ui.tool, db = tool === 'db', redis = tool === 'redis'
    const why = this.buildError(c.x, c.y), x = this.sx(c.x), y = this.sy(c.y)
    g.globalAlpha = 0.6
    if (db) { g.beginPath(); g.ellipse(x, y, 0.9 * S, 0.62 * S, 0, 0, 7) }
    else if (redis) { g.beginPath(); g.roundRect(x - 0.72 * S, y - 0.72 * S, 1.44 * S, 1.44 * S, 0.33 * S) }
    else this.hex(x, y, 0.82 * S)
    g.fillStyle = why ? C.badTop : db ? C.mintTop : redis ? C.violetTop : C.periTop; g.fill()
    g.strokeStyle = why ? C.red : db ? C.mint : redis ? C.violet : C.peri; g.lineWidth = 2.5; g.setLineDash([6, 5]); g.stroke(); g.setLineDash([])
    g.globalAlpha = 1
    this.pill(why ?? `${db ? 'DB 샤드' : redis ? 'Redis' : '서버 추가'} ${this.buildCost()}`, x, y + (this.touch ? -1.9 : 1.25) * S, why ? C.red : db ? C.mintLow : redis ? C.violetLow : C.periLow, why ? 'stop' : undefined)
  }

  private hint(h: Hint, now: number) {
    if (this.drag?.moved) return
    if (!h.drag) return this.hintRing(h, now)
    const a = this.game.nodes.get(h.drag[0]), b = this.game.nodes.get(h.drag[1])
    if (!a || !b) return
    const t = (now / 1700) % 1.25
    const k = Math.min(1, t), e = k * k * (3 - 2 * k)
    const g = this.ctx, S = this.S
    const x = this.sx(a.x) + (this.sx(b.x) - this.sx(a.x)) * e, y = this.sy(a.y) + (this.sy(b.y) - this.sy(a.y)) * e
    g.strokeStyle = C.peri; g.globalAlpha = 0.5; g.lineWidth = S * 0.12; g.lineCap = 'round'
    g.beginPath(); g.moveTo(this.sx(a.x), this.sy(a.y)); g.lineTo(x, y); g.stroke()
    g.globalAlpha = 1
    g.fillStyle = '#fff'; g.strokeStyle = C.periLow; g.lineWidth = 3
    g.beginPath(); g.arc(x, y, S * 0.2, 0, 7); g.fill(); g.stroke()
  }

  private hintRing(h: Hint, now: number) {
    const sel = this.ui.selection
    if (sel && sel.id === (sel.type === 'node' ? h.node : h.link)) return
    const n = h.node ? this.game.nodes.get(h.node) : undefined
    const l = h.link ? this.game.links.get(h.link) : undefined
    const p = l && this.path(l)
    const at = n ?? (p ? this.at(p, 0.5, 0) : h.spot)
    if (!at) return
    const g = this.ctx, S = this.S, x = this.sx(at.x), y = this.sy(at.y), wave = 0.5 + 0.5 * Math.sin(now / 260)
    g.strokeStyle = C.peri; g.lineWidth = 4; g.globalAlpha = 0.45 + 0.55 * wave
    g.beginPath()
    if (h.spot) { g.setLineDash([8, 7]); this.hex(x, y, S * 0.82) }
    else g.arc(x, y, ((n ? this.radius(n) + 0.28 : 0.5) + wave * 0.14) * S, 0, 7)
    g.stroke()
    g.setLineDash([]); g.globalAlpha = 1
  }
}

function curve(ax: number, ay: number, bx: number, by: number, ra: number, rb: number, key: string): Path {
  const d = Math.hypot(bx - ax, by - ay) || 1
  const nx = -(by - ay) / d, ny = (bx - ax) / d
  const bend = Math.min(0.9, d * 0.08)
  const mx = (ax + bx) / 2 + nx * bend, my = (ay + by) / 2 + ny * bend
  const at = (t: number) => ({ x: (1 - t) ** 2 * ax + 2 * (1 - t) * t * mx + t * t * bx, y: (1 - t) ** 2 * ay + 2 * (1 - t) * t * my + t * t * by })
  const t0 = ra / d, t1 = 1 - rb / d, N = 36
  const pts = Array.from({ length: N + 1 }, (_, i) => ({ ...at(t0 + ((t1 - t0) * i) / N), nx: 0, ny: 0 }))
  let len = 0
  pts.forEach((p, i) => {
    const a = pts[Math.max(0, i - 1)], b = pts[Math.min(N, i + 1)], l = Math.hypot(b.x - a.x, b.y - a.y) || 1
    p.nx = -(b.y - a.y) / l; p.ny = (b.x - a.x) / l
    if (i) len += Math.hypot(p.x - pts[i - 1].x, p.y - pts[i - 1].y)
  })
  return { pts, len, key }
}
