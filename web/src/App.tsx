import { Database, Hexagon, MousePointer2, Play, ScrollText, WifiOff, X } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import { Board } from './board'
import { GameClient } from './client'
import { API, DB, SOURCE, type Fault, type Selection, type Tool } from './types'
import { Hud } from './ui/Hud'
import { Help, Menu, ResultModal, RewardModal } from './ui/Overlays'
import { Panel } from './ui/Panel'
import { Start } from './ui/Start'

const KEY = 'serverway.session'
const pref = (k: string, d: boolean) => (localStorage.getItem(k) ?? String(d)) === 'true'

export function App() {
  const [game, setGame] = useState<GameClient | null>(null)
  const [resumeId, setResumeId] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const check = async () => {
    const id = sessionStorage.getItem(KEY)
    setResumeId(id && (await GameClient.exists(id).catch(() => false)) ? id : null)
  }
  useEffect(() => { check() }, [])

  const start = async (seed?: number) => {
    setBusy(true); setError('')
    try {
      const id = await GameClient.create(seed)
      sessionStorage.setItem(KEY, id)
      setGame(new GameClient(id))
    } catch (e) { setError((e as Error).message || '게임 서버에 연결할 수 없습니다') } finally { setBusy(false) }
  }
  if (!game) return <Start onStart={start} busy={busy} error={error} resume={resumeId ? () => setGame(new GameClient(resumeId)) : null} />
  return <Play1 key={game.id} g={game} onHome={() => { game.close(); setGame(null); check() }} />
}

function Play1({ g, onHome }: { g: GameClient; onHome: () => void }) {
  useSyncExternalStore(g.subscribe, g.getVersion)
  const cv = useRef<HTMLCanvasElement>(null)
  const board = useRef<Board | null>(null)
  const [sel, setSel] = useState<Selection>(null)
  const [tool, setTool] = useState<Tool>('connect')
  const [confirm, setConfirm] = useState(false)
  const [menu, setMenu] = useState(false)
  const [help, setHelp] = useState(false)
  const [tutorial, setTut] = useState(() => pref('serverway.tutorial', true))
  const [log, setLogOn] = useState(() => pref('serverway.log', true))
  const setTutorial = (b: boolean) => { setTut(b); localStorage.setItem('serverway.tutorial', String(b)) }
  const setLog = (b: boolean) => { setLogOn(b); localStorage.setItem('serverway.log', String(b)) }
  const select = (s: Selection) => { setSel(s); setConfirm(false) }

  useEffect(() => {
    const b = new Board(cv.current!, g)
    b.onSelect = select
    b.onTool = setTool
    board.current = b
    return () => { b.destroy(); g.close() }
  }, [g])
  useEffect(() => { select(null); setTool('connect') }, [g.resets])

  const h = g.ready ? g.head : null
  const step = useTutorial(g)
  if (board.current) board.current.ui = { selection: sel, tool, hint: tutorial && step?.hint ? step.hint : null }

  // 메뉴가 열리거나 창이 가려지면 게임을 멈춘다.
  const overlay = menu || help
  useEffect(() => { if (overlay && g.ready && !g.head.paused && !g.head.over) g.send('pause', { on: true }) }, [overlay])
  useEffect(() => {
    const hide = () => { if (document.hidden && g.ready && g.status === 'open' && !g.head.paused && !g.head.over) g.send('pause', { on: true }) }
    document.addEventListener('visibilitychange', hide)
    return () => document.removeEventListener('visibilitychange', hide)
  }, [g])

  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (!g.ready || e.target instanceof HTMLInputElement || e.ctrlKey || e.metaKey) return
      if (e.key === 'Escape') {
        if (help) setHelp(false)
        else if (menu) setMenu(false)
        else if (confirm) setConfirm(false)
        else if (tool !== 'connect') setTool('connect')
        else { board.current?.cancel(); select(null) }
        return
      }
      if (menu || help || g.head.reward || g.head.over || e.repeat) return
      if (e.key === ' ') { e.preventDefault(); (document.activeElement as HTMLElement | null)?.blur(); g.send('pause', { on: !g.head.paused }) }
      else if (e.key === 's' || e.key === 'S') g.send('speed', { speed: (g.head.speed % 3) + 1 })
      else if (e.key === '1') setTool('connect')
      else if (e.key === '2') setTool('api')
      else if (e.key === '3') setTool('db')
      else if ((e.key === 'u' || e.key === 'U') && sel) g.send(sel.type === 'node' ? 'upgradeNode' : 'upgradeLink', { id: sel.id })
      else if ((e.key === 'Delete' || e.key === 'Backspace') && sel) setConfirm(true)
    }
    window.addEventListener('keydown', key)
    return () => window.removeEventListener('keydown', key)
  }, [g, sel, tool, confirm, menu, help])

  useEffect(() => {
    if (!g.notice) return
    const id = g.notice.id
    const t = setTimeout(() => { if (g.notice?.id === id) { g.notice = null; g.version++; setSel((s) => s) } }, 3200)
    return () => clearTimeout(t)
  }, [g.notice?.id])

  const focusFault = (f: Fault) => select(f.node ? { type: 'node', id: f.node } : f.link ? { type: 'link', id: f.link } : null)
  const restart = (seed?: number) => { setMenu(false); g.restart(seed) }
  const count = (k: number) => [...g.nodes.values()].filter((n) => n.kind === k).length

  return (
    <div className="game">
      <canvas ref={cv} aria-label="게임 보드" />
      {h && <>
        <Hud g={g} onMenu={() => setMenu(true)} onFocus={focusFault} />
        <Panel g={g} sel={sel} onClose={() => select(null)} confirm={confirm} setConfirm={setConfirm} />
        <div className="dock" role="toolbar" aria-label="건설 도구">
          <button aria-pressed={tool === 'connect'} onClick={() => setTool('connect')}><MousePointer2 size={20} aria-hidden /><span><b>선택 / 연결</b>드래그로 링크</span><kbd>1</kbd></button>
          <button aria-pressed={tool === 'api'} className={h.budget < h.apiCost ? 'poor' : ''} onClick={() => setTool('api')}><Hexagon size={20} aria-hidden /><span><b>API 추가</b>비용 {h.apiCost}, {count(API)}대</span><kbd>2</kbd></button>
          <button aria-pressed={tool === 'db'} className={h.budget < h.dbCost ? 'poor' : ''} onClick={() => setTool('db')}><Database size={20} aria-hidden /><span><b>DB 샤드 추가</b>비용 {h.dbCost}, {count(DB)}대</span><kbd>3</kbd></button>
        </div>
        {log && <LogFeed g={g} onClose={() => setLog(false)} />}
        {!log && <button className="logopen" onClick={() => setLog(true)} aria-label="요청 로그 보기"><ScrollText size={17} /></button>}
        {tutorial && step && !h.reward && !h.over && <div className="tip" role="status"><p>{step.text}</p>{step.done && <button onClick={step.done}>확인</button>}<button className="x" aria-label="안내 끄기" onClick={() => setTutorial(false)}><X size={14} /></button></div>}
        {h.paused && !h.over && !h.reward && !overlay && (
          <div className="pausebar">
            <button className="big primary" onClick={() => g.send('pause', { on: false })}><Play size={18} aria-hidden />{h.started ? '계속하기' : '시작'}</button>
            <span>{h.pauseReason || (h.started ? '멈춘 동안에도 건설하고 연결할 수 있습니다' : '시작하기 전에 미리 연결해 둘 수 있습니다')}</span>
          </div>
        )}
        <RewardModal g={g} />
        <ResultModal g={g} onHome={onHome} onRestart={restart} />
        {menu && <Menu g={g} onClose={() => setMenu(false)} onHome={onHome} onRestart={restart} tutorial={tutorial} setTutorial={setTutorial} log={log} setLog={setLog} onHelp={() => { setMenu(false); setHelp(true) }} />}
        {help && <Help g={g} onClose={() => setHelp(false)} />}
        {g.notice && <div key={g.notice.id} className={`toast ${g.notice.bad ? 'bad' : ''}`} role="status">{g.notice.text}</div>}
      </>}
      {g.status !== 'open' && <div className="veil"><div className="modal small center">
        <WifiOff size={26} aria-hidden />
        {g.status === 'ended' ? <><h2>게임이 만료되었습니다</h2><button className="primary" onClick={onHome}>처음으로</button></> : <><h2>{g.status === 'lost' ? '연결이 끊겼습니다' : '연결하는 중입니다'}</h2><p className="muted">{g.status === 'lost' ? '게임은 멈춰 있습니다. 다시 연결하면 그대로 이어집니다.' : ''}</p></>}
      </div></div>}
    </div>
  )
}

function LogFeed({ g, onClose }: { g: GameClient; onClose: () => void }) {
  const rows = g.logs.slice(-7)
  return (
    <div className="log" aria-label="요청 로그">
      <div><b>요청 로그</b><button className="x" aria-label="로그 닫기" onClick={onClose}><X size={13} /></button></div>
      {rows.length ? rows.map((l) => <p key={l.id} className={l.code === 200 ? '' : 'bad'}><time>{Math.floor(l.at / g.config.tps / 60)}:{String(Math.floor((l.at / g.config.tps) % 60)).padStart(2, '0')}</time><b>{l.code}</b>{l.text}</p>) : <p>아직 오간 트래픽이 없습니다</p>}
    </div>
  )
}

interface Step { text: string; hint?: 'src' | 'db'; done?: () => void }

// 안내는 실제 게임 상태를 보고 다음 단계로 넘어간다.
function useTutorial(g: GameClient): Step | null {
  const [seen, setSeen] = useState<Record<string, boolean>>({})
  useEffect(() => setSeen({}), [g.resets])
  return useMemo(() => {
    if (!g.ready) return null
    const ok = (k: string) => () => setSeen((s) => ({ ...s, [k]: true }))
    const nodes = [...g.nodes.values()], links = [...g.links.values()], h = g.head
    const linked = (from: number, to: number) => links.some((l) => g.nodes.get(l.from)?.kind === from && g.nodes.get(l.to)?.kind === to)
    if (!linked(SOURCE, API)) return { text: '발생지(주황 원)를 누른 채 API(육각형)까지 끌어 놓으세요. 트래픽이 지나갈 길이 생깁니다.', hint: 'src' }
    if (!linked(API, DB)) return { text: '이번에는 API에서 DB(초록 원판)로 끌어 연결하세요. 요청은 DB까지 다녀와야 합니다.', hint: 'db' }
    if (!h.started) return { text: '길이 완성됐습니다. 시작을 누르면 첫 트래픽이 출발합니다.' }
    if (h.completed < 2) return { text: '동그라미는 나가는 요청, 초록 네모는 돌아오는 응답입니다. 발생지에 돌아오면 완료입니다.' }
    if (h.faults.some((f) => f.state === 'notice') && !seen.fault) return { text: '장애가 예고됐습니다. 남은 시간 동안 다른 API를 키우거나 우회 링크를 만들어 두세요.', done: ok('fault') }
    const lone = nodes.find((n) => n.kind === SOURCE && !n.routed)
    if (lone) return { text: `발생지 ${lone.index}이 아직 연결되지 않았습니다. 가까운 API로 끌어 연결하세요.` }
    const server = nodes.find((n) => n.kind !== SOURCE && (n.state === 'busy' || n.state === 'full') && !(n.stuck > 0 && n.stuck >= n.slots))
    if (server && !seen.jam) return { text: `${server.kind === API ? `API ${server.index}` : `DB ${server.index}`}에 구슬이 쌓이고 있습니다. 밀린 줄의 맨 앞이 병목입니다. 눌러서 업그레이드하거나${server.kind === API ? ' API를 추가하세요.' : ' DB 샤드를 추가하세요. API를 늘려도 DB 병목은 풀리지 않습니다.'}`, done: ok('jam') }
    if (links.some((l) => l.state === 'full') && !seen.link) return { text: '빨간 링크는 전송 용량이 한계입니다. 눌러서 업그레이드하거나 링크를 하나 더 만드세요.', done: ok('link') }
    if (h.completed >= 2 && h.tick < 60 * g.config.tps && !seen.grow) return { text: '트래픽은 점점 늘어납니다. 노드나 링크를 누르면 여유가 얼마나 있는지 볼 수 있습니다.', done: ok('grow') }
    return null
  }, [g, g.version, seen])
}
