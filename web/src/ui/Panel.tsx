import { ArrowUp, Cable, HardDrive, Trash2, X } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import type { GameClient } from '../client'
import { API, CACHE, DB, FAIL, SOURCE, nodeName, perSec, type GLink, type GNode, type Selection } from '../types'

export function Panel({ g, sel, onClose, confirm, setConfirm }: { g: GameClient; sel: Selection; onClose: () => void; confirm: boolean; setConfirm: (b: boolean) => void }) {
  const n = sel?.type === 'node' ? g.nodes.get(sel.id) : undefined
  const l = sel?.type === 'link' ? g.links.get(sel.id) : undefined
  const gone = !!sel && !n && !l
  useEffect(() => { if (gone) onClose() }, [gone])
  if (!n && !l) return null
  return (
    <aside className="panel">
      <button className="x" onClick={onClose} aria-label="선택 해제"><X size={16} /></button>
      {n ? <NodeInfo g={g} n={n} /> : <LinkInfo g={g} l={l!} />}
      <Fails fail={(n ?? l)!.fail} />
      {n && n.kind !== SOURCE && !n.draining && <Remove g={g} what={n.kind === API ? '이 서버' : n.kind === CACHE ? '이 Redis' : '이 DB'} refund={n.refund} body="새 요청을 막고, 안에 있는 요청의 응답이 모두 돌아온 뒤 사라집니다." run={() => g.send('removeNode', { id: n.id })} confirm={confirm} setConfirm={setConfirm} />}
      {l && !l.draining && <Remove g={g} what="이 링크" refund={l.refund} body="새 요청을 막고, 이미 나간 요청의 응답이 모두 돌아온 뒤 사라집니다." run={() => g.send('removeLink', { id: l.id })} confirm={confirm} setConfirm={setConfirm} />}
      {(n?.draining || l?.draining) && <p className="muted">철거 중입니다. 왕복 중인 요청 {(n ?? l)!.pending}개가 끝나면 사라집니다.</p>}
    </aside>
  )
}

function Row({ k, v, w }: { k: string; v: ReactNode; w?: boolean }) { return <div className="kv"><dt>{k}</dt><dd className={w ? 'bad' : ''}>{v}</dd></div> }

function Meter({ label, value, max, tone }: { label: string; value: number; max: number; tone: string }) {
  const p = Math.min(1, value / Math.max(1, max))
  return <div className="meter"><span>{label}<b>{value} / {max}</b></span><div className="bar"><i className={p >= 0.8 ? 'bad' : p >= 0.5 ? 'warn' : tone} style={{ width: `${p * 100}%` }} /></div></div>
}

function Upgrade({ g, cost, level, title, icon, onClick, gains }: { g: GameClient; cost: number; level?: number; title?: string; icon?: ReactNode; onClick: () => void; gains: [string, string][] }) {
  const [busy, setBusy] = useState(false)
  const short = cost - g.head.budget
  return (
    <button className={`upgrade ${title ? 'second' : ''}`} disabled={short > 0 || busy || !!g.head.over} onClick={() => { setBusy(true); onClick(); setTimeout(() => setBusy(false), 250) }}>
      {icon ?? <ArrowUp size={17} aria-hidden />}<b>{title ?? `Lv${level! + 2}로 업그레이드`}</b><em>{short > 0 ? `${short} 부족` : cost}</em>
      <span className="gains">{gains.map(([k, v]) => <span key={k}><small>{k}</small>{v}</span>)}</span>
    </button>
  )
}

function NodeInfo({ g, n }: { g: GameClient; n: GNode }) {
  if (n.kind === SOURCE) return (
    <>
      <h2 className="coral">{nodeName(n)}</h2>
      <p className="muted">요청을 만들고, 응답이 돌아오면 완료됩니다.</p>
      <Meter label="나가지 못한 요청" value={n.queue} max={n.limit} tone="coral" />
      <dl><Row k="지금 트래픽" v={`초당 ${(n.rate / 1000).toFixed(1)}개`} /><Row k="30초 뒤" v={`초당 ${(n.nextRate / 1000).toFixed(1)}개`} w={n.nextRate > n.rate * 1.15} /><Row k="완료" v={n.done} /><Row k="연결 상태" v={n.routed ? '완료까지 이어짐' : '길이 없음'} w={!n.routed} /></dl>
    </>
  )
  const stuck = n.stuck > 0 && n.stuck >= n.slots
  const busy = n.jobs.filter((j) => !j.d).length, st = n.store, b = g.head.bonus
  const full = !!st && st.data >= st.cap
  const cut = Math.min(60, n.kind === API ? b.apiOpt : n.kind === DB ? b.dbIndex : 0)
  return (
    <>
      <h2 className={n.kind === DB ? 'mint' : n.kind === CACHE ? 'violet' : 'peri'}>{nodeName(n)} <small>Lv{n.level + 1}</small></h2>
      <p className={(n.state === 'full' && !stuck) || full ? 'bad' : 'muted'}>
        {n.state === 'down' ? '장애로 멈춰 있습니다.' : n.state === 'slow' ? `장애로 처리 속도가 ${n.speed}%입니다.`
          : stuck ? '처리는 끝났지만 다음으로 보낼 길이 막혔습니다. 원인은 이 앞쪽입니다.' : n.state === 'full' ? '이곳이 병목입니다. 처리 용량이 부족합니다.'
            : full ? `저장 공간이 가득 차서 속도가 ${g.config.storageSlow}%입니다.` : n.state === 'busy' ? '대기열이 쌓이고 있습니다.' : '여유가 있습니다.'}
      </p>
      <Meter label="처리 중 (동시 처리 슬롯)" value={busy} max={n.slots} tone={n.kind === DB ? 'mint' : 'peri'} />
      <Meter label={b.queue ? `대기열 (개선 +${b.queue})` : '대기열'} value={n.queue} max={n.limit} tone="peri" />
      {st && <Meter label={b.storage ? `저장 공간 (개선 +${b.storage}%)` : '저장 공간'} value={st.data} max={st.cap} tone="mint" />}
      <dl>
        <Row k="처리 시간" v={cut ? `${n.procMs}ms (개선 -${cut}%)` : `${n.procMs}ms`} /><Row k="최대 처리량" v={`초당 ${perSec(n)}개`} />
        {n.kind === CACHE && <Row k="적중률" v={`${n.hit}%`} />}
        {n.stuck > 0 && <Row k="보낼 길을 기다림" v={`${n.stuck}개`} w />}
        {n.out > 0 && <Row k="응답 대기" v={`${n.out}개`} />}
        <Row k="처리한 요청" v={n.done} />
        {n.kind === API && g.head.cache > 0 && <Row k="바로 응답" v={`${g.head.cache}%`} />}
      </dl>
      {!n.draining && <Upgrade g={g} cost={n.upgrade} level={n.level} onClick={() => g.send('upgradeNode', { id: n.id })}
        gains={n.next ? [['초당 처리', `${perSec(n)} → ${perSec(n.next)}`], n.kind === CACHE ? ['적중률', `${n.hit}% → ${n.next.hit}%`] : ['대기열', `${n.limit} → ${n.next.limit}`]] : []} />}
      {!n.draining && st && <Upgrade g={g} cost={st.cost} title="저장 공간 늘리기" icon={<HardDrive size={17} aria-hidden />} onClick={() => g.send('storage', { id: n.id })} gains={[['저장 공간', `${st.cap.toLocaleString()} → ${st.next.toLocaleString()}`]]} />}
    </>
  )
}

function LinkInfo({ g, l }: { g: GameClient; l: GLink }) {
  const a = g.nodes.get(l.from), b = g.nodes.get(l.to), sec = (t: number) => `${(t / g.config.tps).toFixed(1)}s`
  return (
    <>
      <h2 className="peri">{l.ws ? 'WebSocket 링크' : '링크'} <small>Lv{l.level + 1}</small></h2>
      <p className="muted">{a && nodeName(a)} <span aria-hidden>⇄</span> {b && nodeName(b)}</p>
      <p className={l.state === 'full' || l.state === 'blocked' ? 'bad' : 'muted'}>
        {l.state === 'blocked' ? '장애로 끊겨 있습니다.' : l.state === 'full' ? '이 링크가 병목입니다. 전송 용량이 부족합니다.' : l.state === 'jam' ? '도착한 곳이 꽉 차서 밀려 있습니다. 원인은 링크가 아니라 그 앞쪽입니다.' : l.state === 'busy' ? '전송량이 한계에 가깝습니다.' : '여유가 있습니다.'}
      </p>
      <Meter label={g.head.bonus.linkRate ? `전송 용량 (초당, 개선 +${g.head.bonus.linkRate}%)` : '전송 용량 (한 차선, 초당)'} value={l.used} max={l.cap} tone="peri" />
      <Meter label="링크 위에 있는 요청" value={l.moving[0]} max={l.room} tone="coral" />
      <dl><Row k="돌아오는 응답" v={l.moving[1]} /><Row k="이동 시간 (편도)" v={sec(l.travel)} /><Row k="나가려고 기다림" v={l.waiting} w={l.waiting > 1} /><Row k="지나간 요청" v={l.passed} /></dl>
      {!l.draining && <Upgrade g={g} cost={l.upgrade} level={l.level} onClick={() => g.send('upgradeLink', { id: l.id })} gains={[['초당 전송', `${l.cap} → ${l.nextCap}`]]} />}
      {l.wsNext && <Upgrade g={g} cost={l.wsNext.cost} title="WebSocket으로 전환" icon={<Cable size={17} aria-hidden />} onClick={() => g.send('websocket', { id: l.id })}
        gains={[['초당 전송', `${l.cap} → ${l.wsNext.cap}`], ['이동 (편도)', `${sec(l.travel)} → ${sec(l.wsNext.travel)}`], ['이 링크의 처리 수익', `${g.config.wsRevenue / 100}배`]]} />}
    </>
  )
}

function Fails({ fail }: { fail: number[] }) {
  if (!fail.some(Boolean)) return null
  return <div className="fails"><span>여기서 실패</span>{fail.map((v, i) => v > 0 && <b key={i}>{FAIL[i].code} {FAIL[i].name} {v}</b>)}</div>
}

function Remove({ g, what, refund, body, run, confirm, setConfirm }: { g: GameClient; what: string; refund: number; body: string; run: () => void; confirm: boolean; setConfirm: (b: boolean) => void }) {
  if (!confirm) return <button className="remove" disabled={!!g.head.over} onClick={() => setConfirm(true)}><Trash2 size={15} aria-hidden />철거 (환급 {refund})</button>
  return (
    <div className="confirm" role="alertdialog" aria-label="철거 확인">
      <b>{what}를 철거할까요?</b><p>{body} 예산 {refund}을 돌려받습니다.</p>
      <div><button onClick={() => setConfirm(false)}>취소</button><button className="danger" autoFocus onClick={() => { run(); setConfirm(false) }}>철거</button></div>
    </div>
  )
}
