import { Cable, Clock, Database, DatabaseZap, HardDrive, Layers, ServerCog, Waypoints, Zap } from 'lucide-react'
import type { ReactNode } from 'react'
import type { GameClient } from '../client'
import type { Opened } from '../types'

// 보상으로 쌓은 개선 목록
export function Boosts({ g }: { g: GameClient }) {
  const b = g.head.bonus
  const list: [ReactNode, string][] = []
  if (b.apiOpt) list.push([<Zap />, `서버 처리 시간 -${Math.min(60, b.apiOpt)}%`])
  if (b.dbIndex) list.push([<Database />, `DB 처리 시간 -${Math.min(60, b.dbIndex)}%`])
  if (b.linkRate) list.push([<Waypoints />, `링크 전송 +${b.linkRate}%`])
  if (b.queue) list.push([<Layers />, `대기열 +${b.queue}칸`])
  if (b.deadline) list.push([<Clock />, `처리 기한 +${b.deadline}초`])
  if (b.cache) list.push([<ServerCog />, `서버 바로 응답 ${b.cache}%`])
  if (b.storage) list.push([<HardDrive />, `저장 공간 +${b.storage}%`])
  if (!list.length) return null
  return <ul className="boosts" aria-label="지금까지의 개선">{list.map(([icon, text]) => <li key={text}><span aria-hidden>{icon}</span>{text}</li>)}</ul>
}

// 성장 단계가 올라 새 기능이 열렸을 때의 안내
export function News({ g, onClose }: { g: GameClient; onClose: () => void }) {
  const n = g.news, c = g.config
  if (!n) return null
  const about: Record<Opened, [ReactNode, string, string]> = {
    storage: [<HardDrive />, '저장 공간', `이제 DB가 요청을 처리할 때마다 데이터가 쌓입니다. 가득 차면 DB 속도가 ${c.storageSlow}%로 떨어집니다. DB를 눌러 저장 공간을 늘리거나 샤드를 추가해 나누세요.`],
    redis: [<DatabaseZap />, 'Redis', `아래에 Redis 추가가 생겼습니다. 서버에서 이으면 요청의 ${c.redisHit}%를 DB 대신 훨씬 빠르게 응답합니다. DB로 가는 링크도 그대로 있어야 합니다.`],
    ws: [<Cable />, 'WebSocket', `클라이언트와 서버 사이의 링크를 누르면 전환할 수 있습니다. ${c.wsSpeed / 100}배 빠르고 ${c.wsRate / 100}배 많이 보내며, 그 링크로 완료한 요청은 처리 수익이 ${c.wsRevenue / 100}배입니다.`],
  }
  return (
    <div className="veil" role="dialog" aria-modal aria-label="새 기능">
      <div className="modal news">
        <h2>{n.rank} 단계로 성장했습니다</h2>
        <p className="muted">새 기능이 열렸습니다.</p>
        {n.opened.map((k) => <div key={k} className="item"><span aria-hidden>{about[k][0]}</span><p><b>{about[k][1]}</b>{about[k][2]}</p></div>)}
        <button className="big primary" autoFocus onClick={onClose}>계속하기</button>
      </div>
    </div>
  )
}
