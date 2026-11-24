import { Activity, Coins, FastForward, Gauge, HeartPulse, Pause, Play, Settings, Star, Timer, TriangleAlert } from 'lucide-react'
import type { GameClient } from '../client'
import { FAULT_TEXT, clockText, nodeName, type Fault } from '../types'

export function Hud({ g, onMenu, onFocus }: { g: GameClient; onMenu: () => void; onFocus: (f: Fault) => void }) {
  const { head: h, config: c } = g
  const sec = h.tick / c.tps
  const hp = Math.ceil((h.health / c.healthMax) * 100)
  const tone = hp > 60 ? 'good' : hp > 30 ? 'warn' : 'bad'
  const left = Math.max(0, Math.ceil((h.phaseEnd - h.tick) / c.tps))
  const prog = Math.min(1, (h.tick - h.phaseAt) / Math.max(1, h.phaseEnd - h.phaseAt))
  return (
    <>
      <div className="rail" role="progressbar" aria-label={`다음 보상까지 ${left}초`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(prog * 100)}><i style={{ width: `${prog * 100}%` }} /></div>
      <div className="hud">
        <div className="word">{h.rankName}<small>{h.nextRank ? `다음 단계까지 ${(h.nextRank - h.completed).toLocaleString()}건` : '최고 단계'}, 시드 {g.seed}</small></div>
        <div className="stats">
          <Stat icon={<Star size={17} />} label="점수" value={h.score.toLocaleString()} />
          <div className={`stat health ${tone}`}>
            <HeartPulse size={17} aria-hidden /><div><span>서비스 상태 {tone === 'good' ? '안정' : tone === 'warn' ? '주의' : '위험'}</span><div className="bar"><i style={{ width: `${hp}%` }} /></div></div><strong>{hp}%</strong>
          </div>
          <Stat icon={<Coins size={17} />} label={`예산 (${h.revenueNeed}건당 1)`} value={String(h.budget)} />
          <Stat icon={<Activity size={17} />} label={h.surge > 100 ? `처리량 (트래픽 ${(h.surge / 100).toFixed(1)}배)` : '처리량'} value={`${(h.rps10 / 10).toFixed(1)}/s`} />
          <Stat icon={<Gauge size={17} />} label="p95 왕복" value={h.p95 ? `${(h.p95 / 1000).toFixed(1)}s` : '-'} warn={h.p95 > h.deadline * 700} />
          <Stat icon={<Timer size={17} />} label={`마라톤 (보상 ${left}초)`} value={clockText(sec)} />
        </div>
        <div className="corner">
          <button onClick={() => g.send('pause', { on: !h.paused })} aria-label={h.paused ? '계속하기' : '일시 정지'} title="Space">{h.paused ? <Play size={19} /> : <Pause size={19} />}</button>
          <button onClick={() => g.send('speed', { speed: (h.speed % 3) + 1 })} aria-label={`속도 ${h.speed}배`} title="S"><FastForward size={18} /><em>{h.speed}x</em></button>
          <button onClick={onMenu} aria-label="설정"><Settings size={18} /></button>
        </div>
      </div>
      <div className="alerts">
        {h.faults.map((f) => <FaultAlert key={f.id} f={f} g={g} onFocus={onFocus} />)}
      </div>
    </>
  )
}

function Stat({ icon, label, value, warn }: { icon: React.ReactNode; label: string; value: string; warn?: boolean }) {
  return <div className={`stat ${warn ? 'bad' : ''}`}>{icon}<div><span>{label}</span><strong>{value}</strong></div></div>
}

function FaultAlert({ f, g, onFocus }: { f: Fault; g: GameClient; onFocus: (f: Fault) => void }) {
  const t = FAULT_TEXT[f.type]
  const active = f.state === 'active'
  const left = Math.max(0, Math.ceil(((active ? f.endAt : f.startAt) - g.head.tick) / g.config.tps))
  const n = f.node ? g.nodes.get(f.node) : undefined
  const l = f.link ? g.links.get(f.link) : undefined
  const ends = l && [g.nodes.get(l.from), g.nodes.get(l.to)]
  const target = n ? nodeName(n) : ends?.[0] && ends[1] ? `${nodeName(ends[0])}에서 ${nodeName(ends[1])} 링크` : '대상'
  return (
    <button className={`fault ${active ? 'active' : ''}`} onClick={() => onFocus(f)}>
      <TriangleAlert size={18} aria-hidden />
      <span><b>{active ? `${t.title} 진행 중` : `${left}초 뒤 ${t.title}`}</b>{target}: {t.effect(f)}, {active ? `${left}초 남음` : `${Math.round((f.endAt - f.startAt) / g.config.tps)}초 동안`}{!active && <em>{t.tip}</em>}</span>
    </button>
  )
}
