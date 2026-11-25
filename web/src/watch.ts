// 사람이 아닌 조작을 조용히 살핀다. 게임을 막지 않고, 끝났을 때 기록을 인정할지만 정한다.
// 모두 브라우저 안에서 보는 것이라 마음먹고 사람처럼 꾸민 조작까지는 걸러내지 못한다.

interface Trace {
  fake: number // 프로그램이 만든 입력
  trap: number // 사람에게 보이지 않는 함정을 건드린 횟수
  silent: number // 화면을 조작하지 않았는데 들어온 명령
  clicks: number; jumps: number // 멀리 떨어진 곳을 누른 횟수와, 그중 커서가 순간 이동한 횟수
  drags: number; lines: number // 손가락으로 끈 횟수와, 그중 자로 그은 듯한 횟수
  taps: number[] // 최근에 손가락으로 누른 시간(ms)
}
const empty = (): Trace => ({ fake: 0, trap: 0, silent: 0, clicks: 0, jumps: 0, drags: 0, lines: 0, taps: [] })

const KEY = 'serverway.view'
const FAR = 60 // 이보다 멀리 옮긴 것만 본다(px)
const TOUCH_COUNTS = false // 손가락 검사는 실제 폰에서 값을 확인한 뒤 켠다

let id = '', t = empty()
let lastInput = 0
let moved = 0, at: { x: number; y: number } | null = null
let path: { x: number; y: number; t: number }[] = []

export function watchGame(game: string) {
  id = game
  t = empty()
  try { const s = JSON.parse(localStorage.getItem(KEY) ?? 'null'); if (s?.id === game) t = { ...t, ...s.t } } catch { /* 새로 센다 */ }
}
export function watchReset() { t = empty(); keep() }
function keep() { if (id) try { localStorage.setItem(KEY, JSON.stringify({ id, t })) } catch { /* 저장하지 못해도 이번 판에는 쓴다 */ } }

export const springTrap = () => { t.trap++; keep() }
export const strangeCommand = () => { t.silent++; keep() }
// 화면에서 나온 명령이면 바로 앞에 조작이 있어야 한다.
export function command() { if (performance.now() - lastInput > 5000) strangeCommand() }

const spread = (v: number[]) => { const m = v.reduce((a, b) => a + b, 0) / v.length; return Math.sqrt(v.reduce((a, b) => a + (b - m) ** 2, 0) / v.length) }

// 자로 그은 듯한 끌기: 신호가 몇 번뿐이거나, 흔들림 없이 똑같은 간격으로 움직였다.
function ruler(all: typeof path) {
  const p = all.filter((q, i) => !i || q.x !== all[i - 1].x || q.y !== all[i - 1].y)
  if (p.length <= 3) return true
  const a = p[0], b = p[p.length - 1], len = Math.hypot(b.x - a.x, b.y - a.y)
  const off = Math.max(...p.map((q) => Math.abs((b.x - a.x) * (a.y - q.y) - (a.x - q.x) * (b.y - a.y)) / len))
  const step = p.slice(1).map((q, i) => Math.hypot(q.x - p[i].x, q.y - p[i].y)), mean = len / step.length
  return off < 1 && spread(step) < mean * 0.15
}

const touchOdd = () => t.drags >= 10 && t.lines >= t.drags * 0.9 && t.taps.length >= 20 && (spread(t.taps) < 4 || t.taps.filter((v) => v < 15).length >= t.taps.length * 0.9)
const mouseOdd = () => t.clicks >= 20 && t.jumps >= t.clicks * 0.9

// 이번 판을 사람이 한 것으로 보기 어려운지
export const odd = () => navigator.webdriver === true || t.trap > 0 || t.fake >= 3 || t.silent >= 3 || mouseOdd() || (TOUCH_COUNTS && touchOdd())

function onDown(e: PointerEvent) {
  path = [{ x: e.clientX, y: e.clientY, t: e.timeStamp }]
  if (e.pointerType !== 'mouse') return
  if (at && Math.hypot(e.clientX - at.x, e.clientY - at.y) >= FAR) { t.clicks++; if (moved <= 2) t.jumps++ }
}

function onMove(e: PointerEvent) {
  const all = e.getCoalescedEvents?.() ?? []
  moved += Math.max(1, all.length) // 느린 기기에서 묶여 온 움직임도 따로 센다
  if (path.length && e.buttons) for (const q of all.length ? all : [e]) path.push({ x: q.clientX, y: q.clientY, t: q.timeStamp })
}

function onUp(e: PointerEvent) {
  const a = path[0]
  if (a && e.pointerType === 'touch') {
    if (Math.hypot(e.clientX - a.x, e.clientY - a.y) >= FAR) { t.drags++; if (ruler([...path, { x: e.clientX, y: e.clientY, t: e.timeStamp }])) t.lines++ }
    else t.taps = [...t.taps.slice(-39), Math.round(e.timeStamp - a.t)]
  }
  path = []
  at = { x: e.clientX, y: e.clientY }
  moved = 0
  keep()
}

export function startWatch() {
  const on = <K extends keyof WindowEventMap>(k: K, fn: (e: WindowEventMap[K]) => void) => window.addEventListener(k, (e) => {
    if (e.isTrusted) lastInput = performance.now()
    else if (k !== 'pointermove' && k !== 'pointerup') { t.fake++; keep() }
    fn(e)
  }, { capture: true, passive: true })
  on('pointerdown', onDown)
  on('pointermove', onMove)
  on('pointerup', onUp)
  on('click', () => {})
  on('keydown', () => {})
  // 개발자 도구에서만 보이는 미끼
  Object.defineProperty(window, 'serverwayDebug', { value: Object.freeze({ setScore: springTrap, addBudget: springTrap, autoPlay: springTrap }) })
  if (new URLSearchParams(location.search).has('check')) showCheck()
}

// 주소 뒤에 ?check를 붙이면 지금 값을 보여 준다.
function showCheck() {
  const box = document.createElement('pre')
  box.style.cssText = 'position:fixed;left:6px;top:64px;z-index:99;margin:0;padding:6px 9px;border-radius:9px;background:rgba(0,0,0,.78);color:#fff;font:11px/1.5 ui-monospace,monospace;pointer-events:none'
  document.body.append(box)
  setInterval(() => {
    const n = t.taps.length
    box.textContent = `끌기 ${t.drags} (직선 ${t.lines})\n누르기 ${n} 편차 ${n ? spread(t.taps).toFixed(1) : '-'}ms\n최근 ${t.taps.slice(-6).join(' ')}\n클릭 ${t.clicks} (순간이동 ${t.jumps})\n가짜 ${t.fake} 함정 ${t.trap} 명령 ${t.silent} 표시 ${navigator.webdriver ? 1 : 0}\n손가락 검사 ${touchOdd() ? '걸림' : '통과'}`
  }, 300)
}
