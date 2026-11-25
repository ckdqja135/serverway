import { Dices, GraduationCap, Play, RotateCw, Trophy } from 'lucide-react'
import { useState } from 'react'
import { clockText, loadBest } from '../types'
import { learned } from './Lesson'
import { Toggles } from './Toggles'
import { useFit } from './fit'

export function Start({ onStart, resume, busy, error }: { onStart: (seed?: number, practice?: boolean) => void; resume: (() => void) | null; busy: boolean; error: string }) {
  const [seed, setSeed] = useState('')
  const best = loadBest(), fit = useFit()
  return (
    <div className="start">
      <div ref={fit} className="startcard">
        <div className="hero">
        <Mark />
        <h1>Serverway</h1>
        <p className="tag">트래픽 하나가 오가는 작은 서버에서 시작해<br />감당할 수 있을 때까지 끝없이 키워 보세요.</p>
        {best && <p className="best"><Trophy size={16} aria-hidden />최고 기록 {best.score.toLocaleString()}점, {clockText(best.seconds)}, {best.rank}</p>}
        </div>
        <div className="starts">
        <div className="row">
          {resume && <button className="big" onClick={resume}><RotateCw size={18} aria-hidden />이어서 하기</button>}
          <button className="big primary" disabled={busy} onClick={() => onStart(Number(seed) || undefined)}><Play size={18} aria-hidden />새 게임</button>
          {learned() && <button className="big" disabled={busy} onClick={() => onStart(Number(seed) || undefined, true)}><GraduationCap size={18} aria-hidden />튜토리얼</button>}
        </div>
        <Toggles />
        {!learned() && <p className="fine">처음에는 튜토리얼부터 시작합니다. 언제든 건너뛸 수 있습니다.</p>}
        <label className="seed"><Dices size={15} aria-hidden />시드
          <input inputMode="numeric" maxLength={6} value={seed} placeholder="비우면 무작위" onChange={(e) => setSeed(e.target.value.replace(/\D/g, ''))} /></label>
        <p className="fine">시드는 보상이나 장애 같은 운을 정하는 번호입니다.<br />번호를 넣으면 친구와 같은 운으로 연습하며, 최고 기록에는 남지 않습니다.</p>
        {error && <p className="err" role="alert">{error}</p>}
        </div>
        <ul className="rules">
          <li><b>왕복</b>요청은 서버와 DB를 거친 뒤 응답으로 돌아와야 완료됩니다.</li>
          <li><b>체증</b>서버나 DB가 꽉 차면 트래픽이 링크 위에 밀려 뒤로 번집니다.</li>
          <li><b>성장</b>트래픽과 클라이언트가 끝없이 늘어납니다. 번 예산으로 서버, DB 샤드, 링크를 늘리세요.</li>
        </ul>
        <p className="fine">화면의 요청과 서버는 모두 게임 안의 가상 객체입니다. 실제 서버나 네트워크에는 아무것도 보내지 않습니다.</p>
      </div>
    </div>
  )
}

function Mark() {
  return (
    <svg className="mark" viewBox="0 0 220 72" aria-hidden>
      <path d="M36 36H184" stroke="var(--tubeE)" strokeWidth="20" /><path d="M36 36H184" stroke="var(--tube)" strokeWidth="15" />
      <circle r="5" fill="#ff8a70"><animateMotion dur="2.2s" repeatCount="indefinite" path="M36 39H184" /></circle>
      <rect x="-4.5" y="-4.5" width="9" height="9" rx="2.5" fill="#2fb79a"><animateMotion dur="2.2s" begin="1.1s" repeatCount="indefinite" path="M184 33H36" /></rect>
      <circle cx="36" cy="36" r="25" fill="var(--coralT)" stroke="#ff8a70" strokeWidth="3" /><circle cx="36" cy="36" r="6" fill="#ff8a70" />
      <path d="M110 6l26 15v30l-26 15-26-15V21z" fill="var(--periT)" stroke="#6c7cff" strokeWidth="3" /><circle cx="100" cy="36" r="8" fill="#fff" /><circle cx="120" cy="36" r="8" fill="#6c7cff" />
      {[46, 36, 26].map((y) => <ellipse key={y} cx="184" cy={y} rx="26" ry="15" fill="var(--goodB)" stroke="#2fb79a" strokeWidth="3" />)}
    </svg>
  )
}
