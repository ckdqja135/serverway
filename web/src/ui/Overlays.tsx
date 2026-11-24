import { BookOpen, CircleX, Coins, Flag, Database, Gauge, HeartPulse, House, Layers, Play, RotateCcw, ServerCog, Shuffle, SkipForward, Trophy, Timer, Waypoints, X, Zap } from 'lucide-react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import type { GameClient } from '../client'
import { FAIL, FAULT_TEXT, clockText, loadBest, type Best } from '../types'
import { WhenToGrow } from './Lesson'

const REWARD: Record<string, { icon: ReactNode; name: string; text: (n: number) => string }> = {
  budget: { icon: <Coins />, name: '클라우드 크레딧', text: (n) => `예산을 ${n} 받습니다` },
  queue: { icon: <Layers />, name: '대기열 확장', text: (n) => `모든 서버와 DB의 대기열이 ${n}칸 늘어납니다` },
  deadline: { icon: <Timer />, name: '타임아웃 완화', text: (n) => `새 요청의 처리 기한이 ${n}초 늘어납니다` },
  linkRate: { icon: <Waypoints />, name: 'Keep-Alive 적용', text: (n) => `모든 링크가 초당 ${n}개 더 보냅니다` },
  health: { icon: <HeartPulse />, name: '핫픽스 배포', text: (n) => `서비스 상태를 ${n / 1000}% 회복합니다` },
  apiOpt: { icon: <Zap />, name: '코드 최적화', text: (n) => `모든 서버의 처리 시간이 ${n}% 줄어듭니다` },
  dbIndex: { icon: <Database />, name: 'DB 인덱스 추가', text: (n) => `DB의 처리 시간이 ${n}% 줄어듭니다` },
  cache: { icon: <ServerCog />, name: '응답 캐시', text: (n) => `요청의 ${n}%는 서버가 DB 없이 바로 응답합니다` },
}

export function RewardModal({ g }: { g: GameClient }) {
  const r = g.head.reward
  const ref = useRef<HTMLButtonElement>(null)
  useEffect(() => { ref.current?.focus() }, [r?.phase])
  if (!r) return null
  return (
    <div className="veil" role="dialog" aria-modal aria-label="구간 보상">
      <div className="modal">
        <h2>{r.phase}번째 구간을 버텼습니다</h2>
        <p className="muted">예산 {r.income}이 들어왔습니다.{g.grew && ' 판도 넓어집니다.'} 개선 작업을 하나 고르세요. 고르는 동안 시간은 멈춥니다.</p>
        <div className="cards">
          {r.options.map((o, i) => (
            <button key={o.id} ref={i === 0 ? ref : undefined} className="card" onClick={() => g.send('reward', { index: i })}>
              <span className="ic">{REWARD[o.id]?.icon}</span><b>{REWARD[o.id]?.name ?? o.id}</b><span>{REWARD[o.id]?.text(o.amount)}</span>
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}

export function ResultModal({ g, onHome, onRestart }: { g: GameClient; onHome: () => void; onRestart: (seed?: number) => void }) {
  const r = g.result
  const [best, setBest] = useState<{ old: Best | null; isNew: boolean } | null>(null)
  useEffect(() => {
    if (!r) return setBest(null)
    const old = loadBest()
    const isNew = r.score > (old?.score ?? 0)
    if (isNew) localStorage.setItem('serverway.best', JSON.stringify({ score: r.score, seconds: r.seconds, rank: r.rank }))
    setBest({ old, isNew })
  }, [r?.seed, r?.score, r?.over])
  if (!r) return null
  const total = r.failures.reduce((a, b) => a + b, 0)
  const rate = r.completed + total ? (r.completed / (r.completed + total)) * 100 : 0
  return (
    <div className="veil" role="dialog" aria-modal aria-label="결과">
      <div className="modal result">
        <span className={`outcome ${r.over === 'done' ? 'good' : 'bad'}`}>{r.over === 'done' ? <Flag size={18} aria-hidden /> : <CircleX size={18} aria-hidden />}{r.over === 'done' ? '마라톤을 마쳤습니다' : '서비스가 멈췄습니다'}</span>
        <h2>{clockText(r.seconds)} 동안 달려 {r.rank}까지 키웠습니다</h2>
        <div className="score">{r.score.toLocaleString()}<small>점</small></div>
        {best && (best.isNew ? <p className="newbest"><Trophy size={17} aria-hidden />최고 기록입니다{best.old ? ` (이전 ${best.old.score.toLocaleString()}점)` : ''}</p> : <p className="muted">최고 기록 {best.old!.score.toLocaleString()}점, {clockText(best.old!.seconds)}</p>)}
        <div className="nums">
          <div><span>완료</span><b>{r.completed.toLocaleString()}</b></div><div><span>성공률</span><b>{rate.toFixed(1)}%</b></div>
          <div><span>최고 처리량</span><b>{(r.peakRps / 10).toFixed(1)}/s</b></div><div><span>클라이언트</span><b>{r.sources}개</b></div>
          <div><span>서버 / DB / 링크</span><b>{r.apis} / {r.dbs} / {r.links}</b></div><div><span>최대 정체</span><b>{r.peakWait}개</b></div>
        </div>
        <h3>실패한 요청 {total}개</h3>
        {FAIL.map((f, i) => (
          <div key={f.code} className="failbar"><span><b>{f.code}</b> {f.name}</span><div className="bar"><i className="bad" style={{ width: total ? `${(r.failures[i] / total) * 100}%` : 0 }} /></div><em>{r.failures[i]}</em></div>
        ))}
        {r.faults.length > 0 && <p className="muted">겪은 장애: {r.faults.map((f) => `${FAULT_TEXT[f.type].title} ${f.count}번 (실패 ${f.failed})`).join(', ')}</p>}
        <p className="advice"><Gauge size={17} aria-hidden />{r.advice}</p>
        <div className="row">
          <button onClick={onHome}><House size={16} aria-hidden />처음으로</button>
          <button onClick={() => onRestart(r.seed)}><RotateCcw size={16} aria-hidden />같은 시드 {r.seed}</button>
          <button className="primary" onClick={() => onRestart()}><Shuffle size={16} aria-hidden />새 시드로 다시</button>
        </div>
      </div>
    </div>
  )
}

export function Menu({ g, onClose, onHome, onRestart, onSkip, tutorial, setTutorial, log, setLog, onHelp }: {
  g: GameClient; onSkip: () => void; onClose: () => void; onHome: () => void; onRestart: (seed?: number) => void; tutorial: boolean; setTutorial: (b: boolean) => void; log: boolean; setLog: (b: boolean) => void; onHelp: () => void
}) {
  const practice = g.config.practice
  return (
    <div className="veil" role="dialog" aria-modal aria-label="설정" onClick={onClose}>
      <div className="modal small" onClick={(e) => e.stopPropagation()}>
        <button className="x" onClick={onClose} aria-label="닫기"><X size={16} /></button>
        <h2>설정</h2>
        {!practice && <label className="check"><input type="checkbox" checked={tutorial} onChange={(e) => setTutorial(e.target.checked)} />안내 말풍선 보기</label>}
        <label className="check"><input type="checkbox" checked={log} onChange={(e) => setLog(e.target.checked)} />요청 로그 보기</label>
        <button onClick={onHelp}><BookOpen size={16} aria-hidden />게임 규칙과 조작법</button>
        {practice ? <button onClick={onSkip}><SkipForward size={16} aria-hidden />튜토리얼 건너뛰고 본 게임 시작</button> : <>
        <button onClick={() => { onClose(); g.send('finish') }}><Flag size={16} aria-hidden />여기서 마치고 기록 남기기</button>
        <button onClick={() => onRestart(g.seed)}><RotateCcw size={16} aria-hidden />같은 시드로 처음부터</button>
        </>}
        <button onClick={onHome}><House size={16} aria-hidden />{practice ? '나가기' : '나가기 (게임은 30분 동안 보관)'}</button>
        <button className="primary" autoFocus onClick={onClose}><Play size={16} aria-hidden />돌아가기</button>
      </div>
    </div>
  )
}

export function Help({ g, onClose }: { g: GameClient; onClose: () => void }) {
  const c = g.config
  return (
    <div className="veil" role="dialog" aria-modal aria-label="게임 규칙과 조작법" onClick={onClose}>
      <div className="modal help" onClick={(e) => e.stopPropagation()}>
        <button className="x" onClick={onClose} aria-label="닫기" autoFocus><X size={16} /></button>
        <h2>언제 무엇을 늘리나</h2>
        <WhenToGrow />
        <h2>게임 규칙</h2>
        <ul>
          <li>요청은 클라이언트, 서버, DB 순서로 처리된 뒤 <b>지나온 링크로 되돌아와야</b> 완료됩니다. 완료 하나에 {c.score}점입니다.</li>
          <li><b>마라톤:</b> 끝나는 시간이 없습니다. 클라이언트와 트래픽, 보드가 계속 늘어나며 서비스 상태가 0이 되면 끝납니다. 8분부터는 전체 트래픽이 점점 더 빠르게 늘어납니다.</li>
          <li><b>판:</b> 구간을 넘길 때마다 판이 넓어져 서버와 DB를 더 지을 수 있습니다.</li>
          <li><b>예산:</b> 구간마다 들어오고, 요청을 {g.head.revenueNeed}건 완료할 때마다 1씩 벌립니다. 서버와 DB가 늘면 운영비 때문에 더 많이 완료해야 합니다.</li>
          <li><b>DB 샤드:</b> DB를 더 지으면 요청을 나눠 처리합니다. 어느 샤드로 가도 된다고 단순하게 만들었으며, 서버에서 샤드로 링크를 이어야 쓰입니다.</li>
          <li><b>경로 선택:</b> 지금 보낼 수 있는 링크 중 이동 시간과 도착지 예상 대기를 더한 값이 가장 짧은 곳으로 갑니다. 같으면 먼저 만든 링크입니다.</li>
          <li><b>처리 용량:</b> 서버와 DB 안의 칸이 동시에 처리하는 수, 둘레의 구슬이 대기열입니다. 대기열이 꽉 차면 링크 위에서 기다립니다.</li>
          <li><b>링크 용량:</b> 한 차선이 초당 보낼 수 있는 수입니다. 가는 차선과 돌아오는 차선은 따로입니다.</li>
          <li><b>실패:</b> 502 경로 없음, 503 클라이언트 대기열 초과, 504 처리 기한({g.head.deadline}초) 초과, 500 장애. 실패 하나에 서비스 상태가 {c.failPenalty / 1000}% 줄고, 장애 중에는 그 {c.faultPercent}%만 줄어듭니다. {c.practice ? '튜토리얼에서는 지지 않습니다.' : `처음 ${c.learnSeconds}초는 지지 않습니다.`}</li>
          <li><b>철거:</b> 새 요청만 막고 이미 나간 요청이 돌아온 뒤 사라집니다. 비용의 {c.refundPercent}%를 돌려받습니다.</li>
          <li><b>장애:</b> 시작 전에 예고합니다. 대상은 많이 쓰는 서버나 링크입니다.</li>
          <li>선이 교차해도 이어지지 않습니다. 실제 네트워크를 단순하게 만든 게임 규칙이며 재시도, 로드밸런서, 데이터 일관성은 다루지 않습니다.</li>
        </ul>
        <h2>조작</h2>
        <table><tbody>
          <tr><th>드래그</th><td>클라이언트에서 서버로, 서버에서 DB로 연결</td></tr><tr><th>클릭</th><td>노드나 링크 선택, 빈 곳은 선택 해제</td></tr>
          <tr><th>두 손가락</th><td>확대와 이동 (휴대폰, 태블릿)</td></tr><tr><th>Space</th><td>일시 정지, 계속</td></tr><tr><th>S</th><td>속도 1x, 2x, 3x</td></tr><tr><th>1 / 2 / 3</th><td>연결 / 서버 추가 / DB 샤드 추가</td></tr>
          <tr><th>U</th><td>선택한 대상 업그레이드</td></tr><tr><th>Delete</th><td>선택한 대상 철거 (확인 후)</td></tr><tr><th>Esc</th><td>도구와 선택 취소</td></tr>
        </tbody></table>
      </div>
    </div>
  )
}
