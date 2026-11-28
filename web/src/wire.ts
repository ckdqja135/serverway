// 게임 규칙이 도는 곳과 주고받는 길. 보통은 브라우저 안의 워커이고, VITE_SERVER를 넣어 빌드하면 Go 서버다.
export interface Wire { send(data: string): void; close(): void }
interface Host {
  create(seed: number, practice: boolean, hard: boolean): Promise<string>
  exists(id: string): Promise<boolean>
  last(): string | null // 이어 할 수 있는 판
  open(id: string, onMessage: (m: any) => void, onClose: (ended: boolean) => void): Wire
}

export const LOCAL = !import.meta.env.VITE_SERVER

const KEY = 'serverway.session'
const server: Host = {
  async create(seed, practice, hard) {
    const r = await fetch('/api/sessions', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ seed, practice, hard }) })
    const d = await r.json().catch(() => null)
    if (!d) throw new Error('게임 서버에 연결할 수 없습니다')
    if (!r.ok) throw new Error(d.error)
    if (!practice) sessionStorage.setItem(KEY, d.id)
    return d.id
  },
  exists: async (id) => (await fetch(`/api/sessions/${id}`)).ok,
  last: () => sessionStorage.getItem(KEY),
  open(id, onMessage, onClose) {
    const ws = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws?id=${id}`)
    ws.onmessage = (e) => onMessage(JSON.parse(e.data))
    ws.onclose = () => onClose(false)
    return { send: (d) => ws.send(d), close: () => ws.close() }
  },
}

const SAVE = 'serverway.save'
const SAVE_MS = 3000
const read = (): { id: string; save: unknown } | null => { try { return JSON.parse(localStorage.getItem(SAVE) ?? 'null') } catch { return null } }
const store = (v: unknown) => { try { if (v) localStorage.setItem(SAVE, JSON.stringify(v)); else localStorage.removeItem(SAVE) } catch { /* 저장할 수 없는 브라우저에서는 이어 하기만 되지 않는다 */ } }
const spawn = () => new Worker(new URL('./worker.ts', import.meta.url), { type: 'module' })
// 시작 화면을 보는 동안 미리 받아 둔다.
let spare: Worker | null = LOCAL ? spawn() : null

const games = new Map<string, { seed: number; practice: boolean; hard: boolean }>()
const local: Host = {
  async create(seed, practice, hard) {
    const id = `${Date.now()}`
    games.set(id, { seed, practice, hard })
    return id
  },
  exists: async (id) => games.has(id) || read()?.id === id,
  last: () => read()?.id ?? null,
  open(id, onMessage, onClose) {
    const w = spare ?? spawn()
    spare = null
    const saved = read(), game = games.get(id), keep = !game?.practice
    let epoch = 0, over = false
    const ask = () => { if (keep && epoch && !over) w.postMessage(JSON.stringify({ epoch, type: 'save' })) }
    const timer = setInterval(ask, SAVE_MS)
    const end = () => { clearInterval(timer); w.terminate() }
    let closing = false
    w.onmessage = (e) => {
      const m = JSON.parse(e.data)
      if (m.type === 'save') {
        if (!over) store({ id, save: m.save })
        if (closing) end()
        return
      }
      if (m.type === 'ended' || m.type === 'fail') {
        if (m.type === 'ended') { games.delete(id); store(null) }
        end()
        return onClose(m.type === 'ended')
      }
      epoch = m.epoch
      if (m.head) {
        if (m.head.over && !over && keep) store(null)
        over = !!m.head.over
      }
      if (!closing) onMessage(m)
      if (m.type === 'ack') ask()
    }
    w.onerror = () => { end(); onClose(false) }
    w.postMessage(JSON.stringify(saved?.id === id ? { save: saved.save } : game ?? {}))
    return {
      send: (d) => w.postMessage(d),
      close() { closing = true; ask(); setTimeout(end, keep ? 600 : 0) },
    }
  },
}

export const host = LOCAL ? local : server
