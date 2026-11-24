// 배경 음악과 효과음. 음원 파일 없이 브라우저에서 합성한다.
import { getPref } from './prefs'

let ctx: AudioContext | null = null
let music: GainNode, effects: GainNode, echo: DelayNode

const hz = (midi: number) => 440 * 2 ** ((midi - 69) / 12)

interface Tone { type?: OscillatorType; to?: number; attack?: number; wet?: number }
function tone(out: AudioNode, freq: number, at: number, dur: number, vol: number, o: Tone = {}) {
  const osc = ctx!.createOscillator(), g = ctx!.createGain()
  osc.type = o.type ?? 'sine'
  osc.frequency.setValueAtTime(freq, at)
  if (o.to) osc.frequency.exponentialRampToValueAtTime(o.to, at + dur)
  g.gain.setValueAtTime(0, at)
  g.gain.linearRampToValueAtTime(vol, at + (o.attack ?? 0.006))
  g.gain.exponentialRampToValueAtTime(0.0001, at + dur)
  osc.connect(g).connect(out)
  if (o.wet) { const w = ctx!.createGain(); w.gain.value = o.wet; g.connect(w).connect(echo) }
  osc.start(at)
  osc.stop(at + dur + 0.02)
}

// 배경 음악. 여덟 마디를 되풀이하고, 두 번에 한 번은 가락을 쉰다.

const STEP = 60 / 78 / 2 // 8분음표
const BARS = [
  { bass: 38, pad: [57, 61, 64, 66] }, { bass: 35, pad: [57, 59, 62, 66] }, { bass: 43, pad: [57, 59, 62, 66] }, { bass: 45, pad: [57, 59, 64, 67] },
  { bass: 42, pad: [57, 61, 64, 66] }, { bass: 35, pad: [54, 59, 62, 66] }, { bass: 40, pad: [55, 59, 62, 66] }, { bass: 45, pad: [55, 59, 61, 64] },
]
const PLUCK = [0, 2, 1, 3, 2, 1, 3, 2]
const TUNE: Record<number, number> = { 2: 78, 5: 76, 8: 74, 14: 73, 18: 71, 21: 74, 24: 76, 34: 81, 37: 78, 40: 76, 45: 74, 49: 71, 52: 74, 56: 73, 60: 69 }
let step = 0, next = 0

function schedule() {
  if (!ctx || ctx.state !== 'running' || !getPref('bgm')) { next = 0; return }
  if (next < ctx.currentTime) next = ctx.currentTime + 0.05
  for (; next < ctx.currentTime + 0.4; next += STEP, step++) {
    const i = step % 64, bar = BARS[i >> 3], k = i % 8
    if (k === 0) for (const n of bar.pad) tone(music, hz(n), next, STEP * 9, 0.05, { type: 'triangle', attack: 0.9 })
    if (k === 0 || k === 5) tone(music, hz(bar.bass), next, STEP * 3.4, 0.2, { attack: 0.02 })
    tone(music, hz(bar.pad[PLUCK[k]] + 12), next, 0.5, k % 2 ? 0.035 : 0.055, { type: 'triangle', wet: 0.5 })
    if (TUNE[i] && step % 128 < 64) tone(music, hz(TUNE[i]), next, 1.3, 0.075, { attack: 0.02, wet: 0.6 })
  }
}

// 효과음

export type Sfx = 'done' | 'fail' | 'link' | 'build' | 'up' | 'remove' | 'alert' | 'recover' | 'source' | 'reward' | 'over' | 'finish' | 'bad'
const GAP: Partial<Record<Sfx, number>> = { done: 0.11, fail: 0.2, bad: 0.15 } // 한꺼번에 많이 일어나는 소리는 간격을 둔다
const last: Partial<Record<Sfx, number>> = {}
let beat = 0

export function sfx(name: Sfx) {
  if (!ctx || ctx.state !== 'running' || !getPref('sfx')) return
  const t = ctx.currentTime
  if (t - (last[name] ?? -9) < (GAP[name] ?? 0.04)) return
  last[name] = t
  const seq = (notes: number[], gap: number, dur: number, vol: number, o?: Tone) => notes.forEach((n, i) => tone(effects, hz(n), t + i * gap, dur, vol, o))
  switch (name) {
    case 'done': return tone(effects, hz([74, 78, 81, 78, 76][beat++ % 5]), t, 0.13, 0.05)
    case 'fail': return tone(effects, 190, t, 0.2, 0.07, { type: 'sawtooth', to: 95 })
    case 'link': return seq([69, 76], 0.07, 0.16, 0.14, { type: 'triangle' })
    case 'build': tone(effects, 170, t, 0.18, 0.3, { to: 70 }); return seq([74], 0, 0.2, 0.12, { type: 'triangle' })
    case 'up': return seq([69, 74, 78], 0.07, 0.2, 0.13, { type: 'triangle' })
    case 'remove': return tone(effects, 390, t, 0.2, 0.14, { type: 'triangle', to: 150 })
    case 'alert': return seq([76, 72, 76, 72], 0.13, 0.12, 0.06, { type: 'square' })
    case 'recover': return seq([69, 74, 81], 0.1, 0.3, 0.12)
    case 'source': return seq([78, 83], 0.12, 0.45, 0.13, { wet: 0.4 })
    case 'reward': return seq([74, 78, 81, 86], 0.09, 0.4, 0.13, { type: 'triangle', wet: 0.4 })
    case 'over': return seq([69, 66, 62, 57], 0.22, 0.5, 0.16, { type: 'triangle' })
    case 'finish': return seq([74, 78, 81, 86, 90], 0.12, 0.5, 0.14, { type: 'triangle', wet: 0.4 })
    case 'bad': return tone(effects, 130, t, 0.14, 0.16, { type: 'triangle', to: 90 })
  }
}

// 소리는 사용자가 처음 누른 뒤에만 낼 수 있다.
function open() {
  if (!ctx) {
    ctx = new AudioContext()
    const limit = ctx.createDynamicsCompressor(), soft = ctx.createBiquadFilter(), back = ctx.createGain()
    limit.connect(ctx.destination)
    soft.type = 'lowpass'
    soft.frequency.value = 2600
    soft.connect(limit)
    music = ctx.createGain()
    music.connect(soft)
    effects = ctx.createGain()
    effects.connect(limit)
    echo = ctx.createDelay(1)
    echo.delayTime.value = STEP * 0.75
    back.gain.value = 0.32
    echo.connect(back).connect(echo)
    echo.connect(soft)
    setInterval(schedule, 100)
  }
  if (!document.hidden) ctx.resume()
}

// 배경 음악을 끄면 이미 예약된 소리도 바로 줄인다.
export function applySound() {
  if (ctx) music.gain.setTargetAtTime(getPref('bgm') ? 1 : 0, ctx.currentTime, 0.08)
}

export function startSound() {
  for (const e of ['pointerup', 'touchend', 'keydown']) window.addEventListener(e, open, true)
  document.addEventListener('visibilitychange', () => { if (document.hidden) ctx?.suspend(); else if (ctx) ctx.resume() })
}
