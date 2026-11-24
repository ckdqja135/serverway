import { ArrowRight, CircleX, Coins, Database, GraduationCap, Hexagon, House, Play, SkipForward, TriangleAlert, Waypoints, Zap } from 'lucide-react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import type { Hint } from '../board'
import type { GameClient } from '../client'
import { API, DB, SOURCE, nodeName, type GLink, type GNode } from '../types'

const LEARNED = 'serverway.learned'
const MIN_READ = 3500 // 지켜보는 단계가 글을 읽기 전에 넘어가지 않게 하는 시간(ms)
export const learned = () => localStorage.getItem(LEARNED) === 'true'
export const setLearned = () => localStorage.setItem(LEARNED, 'true')

// 튜토리얼 한 단계. run이 참이면 시간이 흐르고 거짓이면 멈춘다. 비우면 플레이어가 정한다.
interface Step {
  id: string
  title: string
  text: (g: GameClient) => string
  run?: boolean
  scene?: [string, number?][]
  hint?: (g: GameClient) => Hint | null
  focus?: string // 강조할 화면 요소. styles.css의 data-focus와 짝이다
  done?: (g: GameClient, sec: number) => boolean // 없으면 다음 버튼으로 넘어간다
}

const nodes = (g: GameClient, kind: number) => [...g.nodes.values()].filter((n) => n.kind === kind && !n.draining)
const between = (g: GameClient, a: number, b: number) => [...g.links.values()].find((l) => l.from === a && l.to === b && !l.draining)
const toDB = (g: GameClient, api: GNode) => nodes(g, DB).some((d) => between(g, api.id, d.id))
const perSec = (n: GNode) => ((n.slots * 1000) / n.procMs).toFixed(1)
const load = (g: GameClient) => (nodes(g, SOURCE).reduce((s, n) => s + n.rate, 0) / 1000).toFixed(1)
const apiLinks = (g: GameClient): GLink[] => [...g.links.values()].filter((l) => !l.draining && g.nodes.get(l.from)?.kind === API)

const apiGrown = (g: GameClient) => nodes(g, API).some((n) => n.level > 0) || nodes(g, API).length > 1
const linkGrown = (g: GameClient) => apiLinks(g).some((l) => l.level > 0)
const dbGrown = (g: GameClient) => nodes(g, DB).some((n) => n.level > 0) || nodes(g, DB).filter((d) => apiLinks(g).some((l) => l.to === d.id)).length > 1

// 장애가 예고된 API 말고 다른 길이 필요한 곳을 찾는다.
function detour(g: GameClient): { drag: [number, number]; text: string } | null {
  const apis = nodes(g, API)
  const target = g.head.faults[0]?.node ?? apis[0]?.id
  const spare = apis.find((n) => n.id !== target)
  if (!spare) return null
  const lone = nodes(g, SOURCE).find((s) => !apis.some((n) => n.id !== target && between(g, s.id, n.id)))
  if (lone) return { drag: [lone.id, spare.id], text: `${nodeName(lone)}에서 ${nodeName(spare)}로` }
  if (!apis.some((n) => n.id !== target && toDB(g, n))) return { drag: [spare.id, nodes(g, DB)[0].id], text: `${nodeName(spare)}에서 DB로` }
  return null
}

const STEPS: Step[] = [
  {
    id: 'src', title: '길 만들기', run: false,
    text: () => '주황 원은 트래픽이 생기는 클라이언트, 육각형은 서버입니다. 클라이언트를 누른 채 서버까지 끌어 놓으세요.',
    hint: (g) => ({ drag: [nodes(g, SOURCE)[0].id, nodes(g, API)[0].id] }),
    done: (g) => !!between(g, nodes(g, SOURCE)[0].id, nodes(g, API)[0].id),
  },
  {
    id: 'db', title: '길 만들기', run: false,
    text: () => '요청은 서버를 거쳐 DB까지 다녀와야 합니다. 이번에는 서버에서 초록 DB로 끌어 연결하세요.',
    hint: (g) => ({ drag: [nodes(g, API)[0].id, nodes(g, DB)[0].id] }),
    done: (g) => toDB(g, nodes(g, API)[0]),
  },
  {
    id: 'start', title: '서비스 시작', focus: 'start',
    text: () => '길이 이어졌습니다. 시작을 눌러 첫 트래픽을 받아 보세요.',
    done: (g) => g.head.started,
  },
  {
    id: 'trip', title: '왕복 보기', run: true,
    text: () => '동그라미는 나가는 요청, 초록 네모는 돌아오는 응답입니다. 클라이언트로 돌아와야 한 건이 완료됩니다.',
    done: (g) => g.head.completed >= 3,
  },
  {
    id: 'score', title: '점수와 예산', run: false, focus: 'hud',
    text: (g) => `완료하면 점수가 오르고 ${g.head.revenueNeed}건마다 예산을 1 법니다. 실패하면 서비스 상태가 줄고, 0이 되면 게임이 끝납니다. 예산은 서버, DB, 링크를 늘리는 데 씁니다. 튜토리얼에서는 떨어지면 다시 채워 드립니다.`,
  },
  {
    id: 'source', title: '새 클라이언트가 생겼을 때', run: false, scene: [['source']],
    text: (g) => nodes(g, SOURCE).length < 2 ? '새 클라이언트가 나타납니다.' : '시간이 지나면 클라이언트가 늘어납니다. 본 게임에서는 점선 원으로 먼저 예고합니다. 연결하지 않으면 요청이 502로 실패하니, 새 클라이언트를 서버로 연결하세요.',
    hint: (g) => { const s = nodes(g, SOURCE).find((n) => !n.routed); return s ? { drag: [s.id, nodes(g, API)[0].id] } : null },
    done: (g) => nodes(g, SOURCE).length > 1 && nodes(g, SOURCE).every((n) => n.routed),
  },
  {
    id: 'jam', title: '트래픽이 늘어납니다', run: true, scene: [['traffic', 2600]],
    text: () => '육각형 안의 칸은 동시에 처리하는 수, 둘레의 구슬은 기다리는 요청입니다. 서버 둘레에 구슬이 쌓이는 것을 지켜보세요.',
    done: (g, sec) => nodes(g, API).some((n) => n.state === 'full') || apiGrown(g) || sec > 15,
  },
  {
    id: 'api', title: '서버를 늘려야 할 때', run: false, focus: 'upgrade',
    text: (g) => `서버의 칸이 모두 차고 구슬이 쌓이면 서버가 부족한 것입니다. 지금은 초당 ${load(g)}개가 들어오는데 ${perSec(nodes(g, API)[0])}개만 처리합니다. 서버를 누르고 업그레이드하세요.`,
    hint: (g) => ({ node: nodes(g, API)[0].id }),
    done: apiGrown,
  },
  {
    id: 'move', title: '병목은 옮겨 다닙니다', run: true,
    text: () => '서버가 빨라지면 그다음으로 좁은 곳이 막힙니다. 이번에는 어디가 빨개지는지 보세요.',
    done: (g, sec) => apiLinks(g).some((l) => l.state === 'full') || linkGrown(g) || sec > 12,
  },
  {
    id: 'link', title: '링크를 늘려야 할 때', run: false, focus: 'upgrade',
    text: () => '링크가 빨개지고 전송 한계가 뜨면, 링크가 한 번에 보낼 수 있는 양이 부족한 것입니다. 서버나 DB를 키워도 풀리지 않습니다. 빨간 링크를 누르고 업그레이드하세요.',
    hint: (g) => { const l = apiLinks(g).find((x) => x.state === 'full') ?? apiLinks(g)[0]; return l ? { link: l.id } : null },
    done: linkGrown,
  },
  {
    id: 'move2', title: '병목은 옮겨 다닙니다', run: true,
    text: () => '이제 요청이 DB로 몰립니다. DB 앞에 줄이 생기는 것을 보세요.',
    done: (g, sec) => nodes(g, DB).some((n) => n.state === 'full') || dbGrown(g) || sec > 15,
  },
  {
    id: 'db2', title: 'DB를 늘려야 할 때', run: false, focus: 'upgrade',
    text: (g) => nodes(g, DB).length > 1 ? '새 DB 샤드는 서버에서 연결해야 일을 시작합니다. 서버에서 새 DB로 끌어 연결하세요.'
      : `DB에 줄이 생기면 그 뒤의 링크와 서버까지 밀립니다. 줄의 맨 앞이 진짜 원인이라 서버를 늘려도 풀리지 않습니다. DB를 누르고 업그레이드하세요. DB 샤드를 추가해 서버에서 연결해도 됩니다.`,
    hint: (g) => { const dbs = nodes(g, DB), d = dbs.find((n) => !apiLinks(g).some((l) => l.to === n.id)); return d ? { drag: [nodes(g, API)[0].id, d.id] } : { node: dbs[0].id } },
    done: dbGrown,
  },
  {
    id: 'relief', title: '줄의 맨 앞부터', run: true,
    text: () => '밀린 줄이 빠집니다. 막히면 줄의 맨 앞을 찾아 그곳을 넓히는 것이 이 게임의 핵심입니다. 노드나 링크를 누르면 어디가 원인인지 알려 줍니다.',
  },
  {
    id: 'notice', title: '장애 예고', run: true, scene: [['traffic', 1800], ['fault']],
    text: () => '서버도 고장 납니다. 장애는 시작하기 전에 화면에 예고됩니다.',
    done: (g, sec) => g.head.faults.length > 0 || sec > 4,
  },
  {
    id: 'spare', title: '서버를 새로 추가할 때', run: false, focus: 'api',
    text: () => '이 서버가 곧 멈춥니다. 서버가 한 대뿐이면 모든 요청이 멈춥니다. 업그레이드로는 막을 수 없으니 한 대 더 필요합니다. 서버 추가를 누르고 점선 자리에 놓으세요.',
    hint: (g) => ({ spot: g.flip ? { x: 5, y: 10 } : { x: 10, y: 5 } }),
    done: (g) => nodes(g, API).length > 1,
  },
  {
    id: 'detour', title: '우회 길 만들기', run: false,
    text: (g) => `새 서버는 연결해야 일합니다. 클라이언트 둘을 새 서버로, 새 서버를 DB로 이으세요.${detour(g) ? ` 다음: ${detour(g)!.text}` : ''}`,
    hint: (g) => { const d = detour(g); return d ? { drag: d.drag } : null },
    done: (g) => nodes(g, API).length > 1 && !detour(g),
  },
  {
    id: 'fault', title: '장애 넘기기', run: true,
    text: (g) => g.head.faults.some((f) => f.state === 'active') ? '서버 하나가 멈췄지만 요청이 다른 서버로 돌아갑니다. 경로는 덜 밀리는 쪽으로 저절로 정해집니다.' : '곧 장애가 시작됩니다. 요청이 어디로 가는지 보세요.',
    done: (g) => g.head.faults.length === 0,
  },
  {
    id: 'reward', title: '구간 보상', run: true, focus: 'reward', scene: [['reward']],
    text: () => '일정 시간을 버틸 때마다 예산과 개선 카드를 받습니다. 고르는 동안 시간은 멈춥니다. 하나를 골라 보세요.',
    done: (g) => g.head.phase > 0 && !g.head.reward,
  },
  {
    id: 'grow', title: '판이 넓어집니다', run: true,
    text: () => '구간을 넘길 때마다 판이 넓어집니다. 새로 생긴 자리에 서버와 DB를 더 짓고 길을 나누세요. 클라이언트도 더 먼 곳에서 들어옵니다.',
  },
]

export interface Lesson { stage: 'intro' | 'steps' | 'end'; step: Step | null; at: number; next: () => void }

// 안내는 실제 게임 상태를 보고 다음 단계로 넘어간다.
export function useLesson(g: GameClient, halted: boolean): Lesson | null {
  const on = g.ready && g.config.practice
  const [at, setAt] = useState(-1) // -1은 소개 화면
  const entered = useRef(-1)
  const from = useRef({ tick: 0, ms: 0 })
  const step = at >= 0 && at < STEPS.length ? STEPS[at] : null
  const open = g.status === 'open'

  useEffect(() => {
    if (!on || !open || !step || entered.current === at) return
    entered.current = at
    from.current = { tick: g.head.tick, ms: performance.now() }
    step.scene?.forEach(([kind, index]) => g.send('scene', { kind, index: index ?? 0 }))
  }, [on, open, at])
  useEffect(() => {
    if (!on || !step?.done || entered.current !== at || (step.run && performance.now() - from.current.ms < MIN_READ)) return
    if (step.done(g, (g.head.tick - from.current.tick) / g.config.tps)) setAt(at + 1)
  }, [on, at, g.version])
  // 설명하는 동안에는 멈추고 지켜보는 단계에서만 흐르게 한다.
  const run = step ? step.run : false
  useEffect(() => {
    if (on && open && !halted && run !== undefined && g.head.paused === run) g.send('pause', { on: !run })
  }, [on, open, halted, run, on && g.head.paused])

  if (!on) return null
  return { stage: at < 0 ? 'intro' : step ? 'steps' : 'end', step, at, next: () => setAt(at + 1) }
}

export function LessonCard({ g, lesson, onSkip }: { g: GameClient; lesson: Lesson; onSkip: () => void }) {
  const { step, at } = lesson
  if (!step) return null
  return (
    <div className="lesson" role="status">
      <div className="lessonbar" aria-hidden><i style={{ width: `${((at + 1) / STEPS.length) * 100}%` }} /></div>
      <div><b>{step.title}</b><small>튜토리얼 {at + 1} / {STEPS.length}</small><button className="skip" onClick={onSkip}><SkipForward size={14} aria-hidden /><span><i>튜토리얼 </i>건너뛰기</span></button></div>
      <div><p>{step.text(g)}</p>{!step.done && <button className="next" onClick={lesson.next}>다음<ArrowRight size={15} aria-hidden /></button>}</div>
    </div>
  )
}

export function LessonIntro({ onNext, onSkip }: { onNext: () => void; onSkip: () => void }) {
  return (
    <div className="veil" role="dialog" aria-modal aria-label="튜토리얼 소개">
      <div className="modal learn">
        <h2>이렇게 진행됩니다</h2>
        <ol className="flow">
          <li><b>연결</b>클라이언트, 서버, DB를 이으면 요청이 다녀옵니다.</li>
          <li><b>성장</b>클라이언트와 트래픽이 끝없이 늘고, 좁은 곳부터 막힙니다.</li>
          <li><b>확장</b>번 예산으로 막힌 곳을 찾아 넓힙니다.</li>
          <li><b>기록</b>실패가 쌓여 서비스 상태가 0이 되면 끝나고 기록이 남습니다.</li>
        </ol>
        <p className="muted">연습용 판에서 무엇을 언제 늘려야 하는지 하나씩 해 봅니다. 3분쯤 걸리고 여기서는 지지 않습니다.</p>
        <div className="row">
          <button className="big" onClick={onSkip}><SkipForward size={17} aria-hidden />튜토리얼 건너뛰기</button>
          <button className="big primary" autoFocus onClick={onNext}><GraduationCap size={18} aria-hidden />튜토리얼 시작</button>
        </div>
      </div>
    </div>
  )
}

export function LessonEnd({ onDone, onHome }: { onDone: () => void; onHome: () => void }) {
  const go = useRef<HTMLButtonElement>(null)
  useEffect(() => go.current?.focus({ preventScroll: true }), [])
  return (
    <div className="veil" role="dialog" aria-modal aria-label="튜토리얼 완료">
      <div className="modal help learn">
        <h2>튜토리얼을 마쳤습니다</h2>
        <p className="muted">아래 표는 게임 중에도 설정의 게임 규칙에서 다시 볼 수 있습니다.</p>
        <WhenToGrow />
        <p>본 게임은 끝나는 시간이 없습니다. 적은 예산으로 작게 시작하고, 막히기 조금 전에 넓히는 것이 좋습니다.</p>
        <div className="row">
          <button className="big" onClick={onHome}><House size={17} aria-hidden />처음으로</button>
          <button ref={go} className="big primary" onClick={onDone}><Play size={18} aria-hidden />본 게임 시작</button>
        </div>
      </div>
    </div>
  )
}

const GROW: [ReactNode, string, string][] = [
  [<Hexagon />, '서버 칸이 다 차고 구슬이 쌓임', '서버를 업그레이드하거나 추가합니다'],
  [<Waypoints />, '링크가 빨갛고 전송 한계가 뜸', '그 링크를 업그레이드하거나 다른 길을 하나 더 잇습니다'],
  [<Database />, 'DB에 줄이 생기고 뒤까지 밀림', 'DB를 업그레이드하거나 샤드를 추가해 서버에서 잇습니다'],
  [<CircleX />, '클라이언트에서 502가 남', '새 클라이언트를 가까운 서버로 연결합니다'],
  [<TriangleAlert />, '장애가 예고됨', '다른 서버와 우회 링크를 미리 준비합니다'],
  [<Zap />, '업그레이드 비용이 비싸짐', '새로 한 대 짓는 쪽과 비교합니다'],
  [<Coins />, '예산이 남음', '다음에 막힐 곳을 미리 넓힙니다'],
]

export function WhenToGrow() {
  return (
    <table className="grow"><thead><tr><th>이럴 때</th><th>이렇게</th></tr></thead>
      <tbody>{GROW.map(([icon, when, how]) => <tr key={when}><th scope="row"><span aria-hidden>{icon}</span>{when}</th><td>{how}</td></tr>)}</tbody></table>
  )
}
