import { ArrowRight, BookOpen, CircleX, Coins, Flag, Database, Gauge, HardDrive, HeartPulse, House, Layers, Play, RotateCcw, ServerCog, Share2, Shuffle, SkipForward, Trophy, Timer, Waypoints, X, Zap } from 'lucide-react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import type { GameClient } from '../client'
import { FAIL, FAULT_TEXT, clockText, loadBest, revenueText, type Best, type Result } from '../types'
import { LOCAL } from '../wire'
import { Boosts } from './Growth'
import { WhenToGrow } from './Lesson'
import { Toggles } from './Toggles'

const REWARD: Record<string, { icon: ReactNode; name: string; text: (n: number) => string }> = {
  budget: { icon: <Coins />, name: '클라우드 크레딧', text: (n) => `예산을 ${n} 받습니다` },
  queue: { icon: <Layers />, name: '대기열 확장', text: (n) => `모든 서버와 DB의 대기열이 ${n}칸 늘어납니다` },
  deadline: { icon: <Timer />, name: '타임아웃 완화', text: (n) => `새 요청의 처리 기한이 ${n}초 늘어납니다` },
  linkRate: { icon: <Waypoints />, name: 'Keep-Alive 적용', text: (n) => `모든 링크가 ${n}% 더 보냅니다` },
  health: { icon: <HeartPulse />, name: '핫픽스 배포', text: (n) => `서비스 상태를 ${n / 1000}% 회복합니다` },
  apiOpt: { icon: <Zap />, name: '코드 최적화', text: (n) => `모든 서버의 처리 시간이 ${n}% 줄어듭니다` },
  dbIndex: { icon: <Database />, name: 'DB 인덱스 추가', text: (n) => `DB의 처리 시간이 ${n}% 줄어듭니다` },
  storage: { icon: <HardDrive />, name: '스토리지 확장', text: (n) => `모든 DB의 저장 공간이 ${n}% 늘어납니다` },
  cache: { icon: <ServerCog />, name: '응답 캐시', text: (n) => `요청의 ${n}%는 서버가 DB 없이 바로 응답합니다` },
}

export function RewardModal({ g }: { g: GameClient }) {
  const r = g.head.reward
  const ref = useRef<HTMLButtonElement>(null)
  useEffect(() => { ref.current?.focus() }, [r?.phase])
  if (!r) return null
  const pick = async (i: number) => {
    const o = r.options[i], a = await g.send('reward', { index: i })
    if (a.ok) g.say(`${REWARD[o.id]?.name}: ${o.gains.map((v) => `${v.label} ${v.from} → ${v.to}`).join(', ')}`)
  }
  return (
    <div className="veil" role="dialog" aria-modal aria-label="구간 보상">
      <div className="modal">
        <h2>{r.phase}번째 구간을 버텼습니다</h2>
        <ul className="earned">
          <li><span>구간 수입</span><b>+{r.income}</b></li>
          <li><span>보유 예산 보너스 ({r.held}의 {g.head.interestPercent}%{r.interest === r.income && ', 최대'})</span><b>+{r.interest}</b></li>
          <li><span>처리 수익</span><b>{(r.mul / 100).toFixed(1)}배</b></li>
        </ul>
        <p className="muted">{g.grew && '판도 넓어집니다. '}개선 작업을 하나 고르세요. 고르는 동안 시간은 멈춥니다.</p>
        <div className="cards">
          {r.options.map((o, i) => (
            <button key={o.id} ref={i === 0 ? ref : undefined} className="card" onClick={() => pick(i)}>
              <span className="ic">{REWARD[o.id]?.icon}</span><b>{REWARD[o.id]?.name ?? o.id}</b><span>{REWARD[o.id]?.text(o.amount)}</span>
              {o.gains.map((v) => <span key={v.label} className="delta"><small>{v.label}</small><span>{v.from}<ArrowRight size={14} aria-hidden /><b>{v.to}</b></span></span>)}
            </button>
          ))}
        </div>
        <Boosts g={g} />
      </div>
    </div>
  )
}

// 폰에서는 공유 창을 열고, 그 밖에서는 복사한다. 앱 안의 브라우저는 공유와 클립보드를 막기도 해서 예전 복사 방식까지 쓴다.
async function shareResult(r: Result): Promise<string> {
  const text = `[Serverway] ${r.score.toLocaleString()}점\n${clockText(r.seconds)} 동안 버텨 ${r.rank}까지 키웠습니다!\n당신의 관리능력을 테스트해보세요!\n${import.meta.env.VITE_SITE_URL}`
  const copied = '기록을 복사했습니다. 카톡이나 메신저에 붙여 넣으세요.'
  if (navigator.share && matchMedia('(pointer: coarse)').matches) {
    try { await navigator.share({ text }); return '' } catch (e) { if ((e as Error).name === 'AbortError') return '' }
  }
  try { await navigator.clipboard.writeText(text); return copied } catch { /* 아래 방식으로 복사한다 */ }
  const t = document.createElement('textarea')
  t.value = text
  document.body.append(t)
  t.select()
  const ok = document.execCommand('copy')
  t.remove()
  return ok ? copied : '이 브라우저에서는 복사할 수 없습니다. 다른 브라우저에서 열어 주세요.'
}

export function ResultModal({ g, onHome, onRestart }: { g: GameClient; onHome: () => void; onRestart: (seed?: number) => void }) {
  const r = g.result
  const [best, setBest] = useState<{ old: Best | null; isNew: boolean } | null>(null)
  const [note, setNote] = useState('')
  useEffect(() => {
    setNote('')
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
        <div className="half">
        <span className={`outcome ${r.over === 'done' ? 'good' : 'bad'}`}>{r.over === 'done' ? <Flag size={18} aria-hidden /> : <CircleX size={18} aria-hidden />}{r.over === 'done' ? '마라톤을 마쳤습니다' : '서비스가 멈췄습니다'}</span>
        <h2>{clockText(r.seconds)} 동안 달려 {r.rank}까지 키웠습니다</h2>
        <div className="score">{r.score.toLocaleString()}<small>점</small></div>
        {best && (best.isNew ? <p className="newbest"><Trophy size={17} aria-hidden />최고 기록입니다{best.old ? ` (이전 ${best.old.score.toLocaleString()}점)` : ''}</p> : best.old && <p className="muted">최고 기록 {best.old.score.toLocaleString()}점, {clockText(best.old.seconds)}</p>)}
        <div className="nums">
          <div><span>완료</span><b>{r.completed.toLocaleString()}</b></div><div><span>성공률</span><b>{rate.toFixed(1)}%</b></div>
          <div><span>최고 처리량</span><b>{(r.peakRps / 10).toFixed(1)}/s</b></div><div><span>클라이언트</span><b>{r.sources}개</b></div>
          <div><span>서버 / DB / 링크</span><b>{r.apis} / {r.dbs} / {r.links}</b></div><div><span>최대 정체</span><b>{r.peakWait}개</b></div>
        </div>
        </div>
        <div className="half">
        <h3>실패한 요청 {total}개</h3>
        <div className="failbars">{FAIL.map((f, i) => (
          <div key={f.code} className="failbar"><span><b>{f.code}</b> {f.name}</span><div className="bar"><i className="bad" style={{ width: total ? `${(r.failures[i] / total) * 100}%` : 0 }} /></div><em>{r.failures[i]}</em></div>
        ))}</div>
        {r.faults.length > 0 && <p className="muted">겪은 장애: {r.faults.map((f) => `${FAULT_TEXT[f.type].title} ${f.count}번 (실패 ${f.failed})`).join(', ')}</p>}
        <p className="advice"><Gauge size={17} aria-hidden />{r.advice}</p>
        </div>
        <div className="row">
          <button onClick={onHome}><House size={16} aria-hidden />처음으로</button>
          <button onClick={() => onRestart(r.seed)}><RotateCcw size={16} aria-hidden />같은 시드 {r.seed}</button>
          <button onClick={async () => setNote(await shareResult(r))}><Share2 size={16} aria-hidden />기록 공유</button>
          <button className="primary" onClick={() => onRestart()}><Shuffle size={16} aria-hidden />새 시드로 다시</button>
        </div>
        {note && <p role="status">{note}</p>}
        <p className="fine">시드는 운을 정하는 번호입니다.<span> 같은 시드는 이번 판과 같은 운으로, 새 시드는 새로운 운으로 시작합니다.</span></p>
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
        <Toggles />
        {!practice && <label className="check"><input type="checkbox" checked={tutorial} onChange={(e) => setTutorial(e.target.checked)} />안내 말풍선 보기</label>}
        <label className="check"><input type="checkbox" checked={log} onChange={(e) => setLog(e.target.checked)} />요청 로그 보기</label>
        <button onClick={onHelp}><BookOpen size={16} aria-hidden />게임 규칙과 조작법</button>
        {practice ? <button onClick={onSkip}><SkipForward size={16} aria-hidden />튜토리얼 건너뛰고 본 게임 시작</button> : <>
        <button onClick={() => { onClose(); g.send('finish') }}><Flag size={16} aria-hidden />여기서 마치고 기록 남기기</button>
        <button onClick={() => onRestart(g.seed)}><RotateCcw size={16} aria-hidden />같은 시드로 처음부터</button>
        <p className="fine">시드({g.seed})는 이 판의 운을 정하는 번호입니다. 지금 판을 버리고 같은 운으로 다시 시작합니다.</p>
        </>}
        <button onClick={onHome}><House size={16} aria-hidden />{practice ? '나가기' : LOCAL ? '나가기 (이어서 할 수 있음)' : '나가기 (게임은 30분 동안 보관)'}</button>
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
          <li><b>예산:</b> 구간마다 들어오고 구간이 오를수록 많아집니다. 구간을 넘길 때 가진 예산의 {g.head.interestPercent}%를 보너스로 받습니다(그 구간 수입까지). 요청을 완료해도 벌리며 지금은 {revenueText(g.head)}입니다. 서버와 DB가 늘면 운영비 때문에 더 완료해야 하지만, 구간이 오를수록 수익이 커집니다.</li>
          <li><b>저장 공간:</b> 성장 서비스부터 DB가 처리할 때마다 데이터가 쌓입니다. 가득 차면 속도가 {c.storageSlow}%로 떨어집니다. 저장 공간을 늘리거나 샤드를 추가해 나눕니다.</li>
          <li><b>Redis:</b> 국민 서비스부터 지을 수 있습니다. 서버에서 이으면 요청의 일부를 DB 대신 응답합니다. DB로 가는 링크도 있어야 합니다.</li>
          <li><b>WebSocket:</b> 국민 서비스부터 클라이언트와 서버 사이 링크를 전환할 수 있습니다. {c.wsSpeed / 100}배 빠르고 {c.wsRate / 100}배 많이 보내며 처리 수익이 {c.wsRevenue / 100}배입니다.</li>
          <li><b>DB 샤드:</b> DB를 더 지으면 요청을 나눠 처리합니다. 어느 샤드로 가도 된다고 단순하게 만들었으며, 서버에서 샤드로 링크를 이어야 쓰입니다.</li>
          <li><b>경로 선택:</b> 지금 보낼 수 있는 링크 중 이동 시간과 도착지 예상 대기를 더한 값이 가장 짧은 곳으로 갑니다. 같으면 먼저 만든 링크입니다.</li>
          <li><b>처리 용량:</b> 서버와 DB 안의 칸이 동시에 처리하는 수, 둘레의 구슬이 대기열입니다. 대기열이 꽉 차면 링크 위에서 기다립니다.</li>
          <li><b>링크 용량:</b> 한 차선이 초당 보낼 수 있는 수입니다. 가는 차선과 돌아오는 차선은 따로입니다.</li>
          <li><b>실패:</b> 502 경로 없음, 503 클라이언트 대기열 초과, 504 처리 기한({g.head.deadline}초) 초과, 500 장애. 실패 하나에 서비스 상태가 {c.failPenalty / 1000}% 줄고, 장애 중에는 그 {c.faultPercent}%만 줄어듭니다. {c.practice ? '튜토리얼에서는 지지 않습니다.' : `처음 ${c.learnSeconds}초는 지지 않습니다.`}</li>
          <li><b>시드:</b> 보상 후보, 장애, 나중에 생기는 클라이언트 위치 같은 운을 정하는 번호입니다. 같은 시드에 같은 플레이면 똑같이 진행되고, 다르게 플레이하면 뒤로 갈수록 달라집니다. 하던 판을 계속하는 이어서 하기와는 다릅니다.</li>
          <li><b>철거:</b> 새 요청만 막고 이미 나간 요청이 돌아온 뒤 사라집니다. 비용의 {c.refundPercent}%를 돌려받습니다.</li>
          <li><b>장애:</b> 시작 전에 예고합니다. 대상은 많이 쓰는 서버나 링크입니다.</li>
          <li>선이 교차해도 이어지지 않습니다. 실제 네트워크를 단순하게 만든 게임 규칙이며 재시도, 로드밸런서, 데이터 일관성은 다루지 않습니다.</li>
        </ul>
        <h2>조작</h2>
        <table><tbody>
          <tr><th>드래그</th><td>클라이언트에서 서버로, 서버에서 DB로 연결</td></tr><tr><th>클릭</th><td>노드나 링크 선택, 빈 곳은 선택 해제</td></tr>
          <tr><th>두 손가락</th><td>확대와 이동 (휴대폰, 태블릿)</td></tr><tr><th>Space</th><td>일시 정지, 계속</td></tr><tr><th>S</th><td>속도 1x, 2x, 3x</td></tr><tr><th>1 / 2 / 3 / 4</th><td>연결 / 서버 추가 / DB 샤드 추가 / Redis 추가</td></tr>
          <tr><th>U</th><td>선택한 대상 업그레이드</td></tr><tr><th>Delete</th><td>선택한 대상 철거 (확인 후)</td></tr><tr><th>Esc</th><td>도구와 선택 취소</td></tr>
        </tbody></table>
      </div>
    </div>
  )
}
