import { ArrowRight, Bug, CircleX, Coins, Database, DatabaseZap, FileWarning, GraduationCap, HardDrive, Hexagon, House, LockKeyhole, Play, ShieldAlert, SkipForward, TriangleAlert, Waypoints, Zap } from 'lucide-react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import type { Hint } from '../board'
import type { GameClient } from '../client'
import { API, DB, SOURCE, nodeName, perSec, type Fault, type GLink, type GNode, type Threat } from '../types'
import { useFit } from './fit'

const LEARNED = 'serverway.learned'
const HARD_LEARNED = 'serverway.learned.hard' // 하드 모드의 보안 튜토리얼
const MIN_READ = 3500 // 지켜보는 단계가 글을 읽기 전에 넘어가지 않게 하는 시간(ms)
export const learned = (hard = false) => localStorage.getItem(hard ? HARD_LEARNED : LEARNED) === 'true'
export const setLearned = (hard = false) => localStorage.setItem(hard ? HARD_LEARNED : LEARNED, 'true')

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

// 하드 모드의 보안 튜토리얼. 하드 모드와 같은 판에서 보안 사고를 하나씩 일으켜 막는 법을 해 본다.
const threat = (g: GameClient, t: Threat): Fault | undefined => g.head.faults.find((f) => f.type === t)
const aim = (t: Threat) => (g: GameClient) => { const f = threat(g, t); return f?.node ? { node: f.node } : null }
const handled = (t: Threat) => (g: GameClient) => !threat(g, t) || !!threat(g, t)!.patched

const HARD_STEPS: Step[] = [
  {
    id: 'start', title: '하드 모드', focus: 'start',
    text: () => '하드 모드는 클라이언트 셋, 서버 셋, DB 둘이 이미 이어진 스타트업에서 시작합니다. 처음 45초 보호가 없고, 장애에 더해 보안 사고가 일어납니다. 시작을 눌러 보세요.',
    done: (g) => g.head.started,
  },
  {
    id: 'hack', title: '해킹 예고', run: true, scene: [['traffic', 1500], ['hack']],
    text: () => '보안 사고도 장애처럼 먼저 예고됩니다. 보라색 점선이 공격받을 곳이고, 위쪽 알림에 남은 시간이 나옵니다.',
    done: (g, sec) => !!threat(g, 'hack') || sec > 4,
  },
  {
    id: 'hack-patch', title: '보안 패치로 막기', run: false, focus: 'patch',
    text: (g) => `해킹은 취약한 서버를 뚫고 가진 예산의 ${threat(g, 'hack')?.percent ?? 30}%를 빼앗아 갑니다. 공격받을 서버를 누르고 보안 패치를 하세요. 패치하는 ${g.config.patchSec}초 동안 그 서버는 멈추니, 다른 서버가 요청을 받아 줄 수 있을 때 하는 것이 좋습니다.`,
    hint: aim('hack'), done: handled('hack'),
  },
  {
    id: 'hack-block', title: '공격 막기', run: true,
    text: () => '패치를 마친 곳은 초록색으로 바뀌고, 공격이 와도 막아 냅니다. 패치는 공격 전에만 할 수 있습니다.',
    done: (g) => !threat(g, 'hack'),
  },
  {
    id: 'ddos', title: 'DDoS 예고', run: true, scene: [['ddos']],
    text: () => '이번에는 DDoS입니다. 한 클라이언트에서 가짜 요청이 쏟아질 예정입니다.',
    done: (g, sec) => !!threat(g, 'ddos') || sec > 4,
  },
  {
    id: 'ddos-patch', title: 'DDoS 차단 설정', run: false, focus: 'patch',
    text: () => '막지 못하면 회색 가짜 요청이 링크와 서버를 차지해 진짜 요청이 밀려 실패합니다. 가짜 요청은 점수도 예산도 주지 않습니다. 노리는 클라이언트를 누르고 DDoS 차단 설정을 하세요. 설정하는 동안 그 클라이언트는 잠시 요청을 보내지 않습니다.',
    hint: aim('ddos'), done: handled('ddos'),
  },
  {
    id: 'ddos-block', title: '공격 막기', run: true,
    text: () => '차단 설정을 마쳤습니다. 막지 못했을 때는 그 클라이언트의 링크와 서버를 넉넉히 두면 가짜 요청을 버텨 낼 수 있습니다.',
    done: (g) => !threat(g, 'ddos'),
  },
  {
    id: 'leak', title: '데이터 유출 예고', run: true, scene: [['leak']],
    text: () => '데이터 유출은 DB를 노립니다.',
    done: (g, sec) => !!threat(g, 'leak') || sec > 4,
  },
  {
    id: 'leak-patch', title: 'DB 패치', run: false, focus: 'patch',
    text: (g) => `유출되면 서비스 상태가 한 번에 ${g.config.leakHealth / 1000}% 줄어듭니다. 실패와 달리 1초 상한도 없습니다. DB를 누르고 보안 패치를 하세요. 패치하는 동안 그 DB가 멈추니, 샤드를 둘 이상 두면 언제든 안심하고 패치할 수 있습니다.`,
    hint: aim('leak'), done: handled('leak'),
  },
  {
    id: 'leak-block', title: '공격 막기', run: true,
    text: () => '유출을 막았습니다. 패치 비용은 구간이 오를수록 조금씩 오릅니다.',
    done: (g) => !threat(g, 'leak'),
  },
  {
    id: 'ransom', title: '랜섬웨어 예고', run: true, scene: [['ransom']],
    text: () => '마지막은 랜섬웨어입니다. 이번에는 패치하지 말고 무슨 일이 일어나는지 지켜보세요.',
    done: (g) => !threat(g, 'ransom') || threat(g, 'ransom')!.state === 'active',
  },
  {
    id: 'ransom-pay', title: '잠긴 DB 풀기', run: false, focus: 'patch',
    text: (g) => threat(g, 'ransom') ? `DB가 잠겨 요청을 처리하지 못합니다. 잠긴 DB를 누르고 몸값을 내면 바로 풀립니다. 내지 않으면 ${Math.round((threat(g, 'ransom')!.endAt - threat(g, 'ransom')!.startAt) / g.config.tps)}초 동안 잠겨 있으니, 다른 DB 샤드가 버틸 수 있다면 기다리는 것도 방법입니다.` : '미리 패치해서 랜섬웨어를 막았습니다. 막지 못했다면 몸값을 내고 풀거나 다른 DB로 버텨야 합니다.',
    hint: aim('ransom'), done: (g) => !threat(g, 'ransom'),
  },
  {
    id: 'wrap', title: '정리', run: true,
    text: () => '보안 사고는 공격 전에 패치로 막는 것이 가장 쌉니다. 패치하는 동안 그곳이 멈추므로, 서버와 DB를 여유 있게 두면 언제든 패치할 수 있습니다.',
  },
]

export interface Lesson { stage: 'intro' | 'steps' | 'end'; step: Step | null; at: number; total: number; hard: boolean; next: () => void }

// 안내는 실제 게임 상태를 보고 다음 단계로 넘어간다.
export function useLesson(g: GameClient, halted: boolean): Lesson | null {
  const on = g.ready && g.config.practice
  const hard = on && g.config.hard
  const steps = hard ? HARD_STEPS : STEPS
  const [at, setAt] = useState(-1) // -1은 소개 화면
  const entered = useRef(-1)
  const from = useRef({ tick: 0, ms: 0 })
  const step = at >= 0 && at < steps.length ? steps[at] : null
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
  return { stage: at < 0 ? 'intro' : step ? 'steps' : 'end', step, at, total: steps.length, hard, next: () => setAt(at + 1) }
}

export function LessonCard({ g, lesson, onSkip }: { g: GameClient; lesson: Lesson; onSkip: () => void }) {
  const { step, at, total } = lesson
  if (!step) return null
  return (
    <div className="lesson" role="status">
      <div className="lessonbar" aria-hidden><i style={{ width: `${((at + 1) / total) * 100}%` }} /></div>
      <div><b>{step.title}</b><small>{lesson.hard ? '보안 튜토리얼' : '튜토리얼'} {at + 1} / {total}</small><button className="skip" onClick={onSkip}><SkipForward size={14} aria-hidden /><span><i>튜토리얼 </i>건너뛰기</span></button></div>
      <div><p>{step.text(g)}</p>{!step.done && <button className="next" onClick={lesson.next}>다음<ArrowRight size={15} aria-hidden /></button>}</div>
    </div>
  )
}

export function LessonIntro({ onNext, onSkip, hard }: { onNext: () => void; onSkip: () => void; hard: boolean }) {
  const fit = useFit()
  if (hard) return (
    <div className="veil" role="dialog" aria-modal aria-label="보안 튜토리얼 소개">
      <div ref={fit} className="modal learn">
        <h2>하드 모드에 오신 것을 환영합니다</h2>
        <ol className="flow">
          <li><b>시작</b>이미 갖춘 스타트업에서, 보호 시간 없이 시작합니다.</li>
          <li><b>예고</b>보안 사고는 공격 전에 보라색으로 예고됩니다.</li>
          <li><b>방어</b>공격받을 곳에 보안 패치를 하면 막습니다.</li>
          <li><b>대응</b>막지 못하면 예산이나 서비스 상태를 잃습니다.</li>
        </ol>
        <p className="muted">해킹, DDoS, 데이터 유출, 랜섬웨어를 하나씩 겪으며 막는 법을 해 봅니다. 2분쯤 걸리고 여기서는 지지 않습니다.</p>
        <div className="row">
          <button className="big" onClick={onSkip}><SkipForward size={17} aria-hidden />건너뛰고 하드 모드</button>
          <button className="big primary" autoFocus onClick={onNext}><ShieldAlert size={18} aria-hidden />보안 튜토리얼 시작</button>
        </div>
      </div>
    </div>
  )
  return (
    <div className="veil" role="dialog" aria-modal aria-label="튜토리얼 소개">
      <div ref={fit} className="modal learn">
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

export function LessonEnd({ onDone, onHome, hard }: { onDone: () => void; onHome: () => void; hard: boolean }) {
  const go = useRef<HTMLButtonElement>(null), fit = useFit()
  useEffect(() => go.current?.focus({ preventScroll: true }), [])
  if (hard) return (
    <div className="veil" role="dialog" aria-modal aria-label="보안 튜토리얼 완료">
      <div ref={fit} className="modal help learn">
        <h2>보안 튜토리얼을 마쳤습니다</h2>
        <p className="muted">아래 표는 하드 모드 중에도 설정의 게임 규칙에서 다시 볼 수 있습니다.</p>
        <Threats />
        <p>하드 모드는 처음부터 트래픽이 많고 보호 시간이 없습니다. 패치할 예산을 조금 남겨 두세요.</p>
        <div className="row">
          <button className="big" onClick={onHome}><House size={17} aria-hidden />처음으로</button>
          <button ref={go} className="big primary" onClick={onDone}><Play size={18} aria-hidden />하드 모드 시작</button>
        </div>
      </div>
    </div>
  )
  return (
    <div className="veil" role="dialog" aria-modal aria-label="튜토리얼 완료">
      <div ref={fit} className="modal help learn">
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
  [<HardDrive />, 'DB 저장 공간이 차오름', '저장 공간을 늘리거나 샤드를 추가해 나눕니다'],
  [<DatabaseZap />, '국민 서비스가 됨', '서버에 Redis를 잇고, 바쁜 클라이언트 링크를 WebSocket으로 바꿉니다'],
  [<CircleX />, '클라이언트에서 502가 남', '새 클라이언트를 가까운 서버로 연결합니다'],
  [<TriangleAlert />, '장애가 예고됨', '다른 서버와 우회 링크를 미리 준비합니다'],
  [<Zap />, '업그레이드 비용이 비싸짐', '새로 한 대 짓는 쪽과 비교합니다'],
  [<Coins />, '예산이 남음', '다음에 막힐 곳을 미리 넓힙니다'],
]

const THREATS: [ReactNode, string, string, string][] = [
  [<Bug />, '해킹', '서버. 가진 예산의 일부를 빼앗김', '서버를 눌러 보안 패치'],
  [<ShieldAlert />, 'DDoS', '클라이언트. 가짜 요청이 처리 용량을 차지함', '클라이언트를 눌러 차단 설정, 놓쳤다면 링크와 서버를 넉넉히'],
  [<FileWarning />, '데이터 유출', 'DB. 서비스 상태가 한 번에 줄어듦', 'DB를 눌러 보안 패치'],
  [<LockKeyhole />, '랜섬웨어', 'DB. 잠겨서 처리가 멈춤', 'DB를 눌러 보안 패치, 잠기면 몸값을 내거나 다른 샤드로 버티기'],
]

// Threats는 보안 사고와 막는 법이다.
export function Threats() {
  return (
    <table className="grow threats"><thead><tr><th>사고</th><th>노리는 곳과 피해</th><th>막는 법</th></tr></thead>
      <tbody>{THREATS.map(([icon, name, what, how]) => <tr key={name}><th scope="row"><span aria-hidden>{icon}</span>{name}</th><td>{what}</td><td>{how}</td></tr>)}</tbody></table>
  )
}

export function WhenToGrow() {
  return (
    <table className="grow"><thead><tr><th>이럴 때</th><th>이렇게</th></tr></thead>
      <tbody>{GROW.map(([icon, when, how]) => <tr key={when}><th scope="row"><span aria-hidden>{icon}</span>{when}</th><td>{how}</td></tr>)}</tbody></table>
  )
}
