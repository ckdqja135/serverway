import type { GameClient } from '../client'
import { clockText, type Result } from '../types'

export interface Frame { x: number; y: number; w: number; h: number } // 캔버스 안에서 판이 차지하는 자리(px)

const W = 1080, PAD = 56
const KO = '"Gothic A1", sans-serif', NUM = '"Unbounded", "Gothic A1", sans-serif'

// 결과를 그림 한 장으로 그린다. 색은 지금 화면 모드를 따른다. 판이 세로로 길면 왼쪽에 판, 오른쪽에 기록을 둔다.
async function draw(r: Result, g: GameClient, frame: Frame | undefined): Promise<Blob> {
  const total = r.failures.reduce((a, b) => a + b, 0)
  const cells: [string, string][] = [
    ['완료', r.completed.toLocaleString()], ['성공률', `${(r.completed + total ? (r.completed / (r.completed + total)) * 100 : 0).toFixed(1)}%`], ['최고 처리량', `${(r.peakRps / 10).toFixed(1)}/s`],
    ['클라이언트', `${r.sources}개`], ['서버 / DB / 링크', `${r.apis} / ${r.dbs} / ${r.links}`], ['최대 정체', `${r.peakWait}개`],
  ]
  const badge = r.over === 'done' ? '마라톤을 마쳤습니다' : '서비스가 멈췄습니다'
  const title = `${clockText(r.seconds)} 동안 달려 ${r.rank}까지 키웠습니다`
  const tag = g.odd ? 'AI 등의 화면 어뷰징 의심' : g.chosen ? `시드 ${r.seed} 연습` : ''
  const score = r.score.toLocaleString(), call = '당신의 관리능력을 테스트해보세요!', site = new URL(import.meta.env.VITE_SITE_URL).host
  // 글꼴은 쓰는 글자만 나눠 받으므로, 그리기 전에 필요한 글자를 모두 받아 둔다.
  const words = [badge, title, tag, call, '점', ...cells.flat()].join('')
  await Promise.all([...[400, 600, 800].map((k) => document.fonts.load(`${k} 30px "Gothic A1"`, words)), document.fonts.load('600 30px "Unbounded"', `Serverway${score}${cells.flat().join('')}`)])

  const css = getComputedStyle(document.documentElement), v = (k: string) => css.getPropertyValue(`--${k}`).trim()
  const tall = !!frame && frame.h > frame.w, H = tall ? 1150 : 1350
  const cv = document.createElement('canvas')
  cv.width = W; cv.height = H
  const c = cv.getContext('2d')!
  const box = (x: number, y: number, w: number, h: number, rad: number, fill: string) => { c.fillStyle = fill; c.beginPath(); c.roundRect(x, y, w, h, rad); c.fill() }
  // 글자의 실제 높이를 재서 세로 가운데에 놓는다.
  const text = (s: string, x: number, y: number, font: string, color: string, align: CanvasTextAlign = 'left') => {
    c.font = font; c.fillStyle = color; c.textAlign = align; c.textBaseline = 'alphabetic'
    const m = c.measureText(s)
    c.fillText(s, x, y + (m.actualBoundingBoxAscent - m.actualBoundingBoxDescent) / 2)
    return m.width
  }

  c.fillStyle = v('bg'); c.fillRect(0, 0, W, H)
  // 마지막 판
  const by = 120, bh = tall ? 900 : 500, bw = tall ? 480 : W - PAD * 2, game = document.querySelector<HTMLCanvasElement>('.game canvas')
  c.save()
  c.beginPath(); c.roundRect(PAD, by, bw, bh, 36); c.clip()
  c.fillStyle = v('area'); c.fillRect(PAD, by, bw, bh)
  if (game && frame) {
    const k = game.width / game.clientWidth, wide = frame.w / frame.h > bw / bh
    const sw = wide ? frame.h * (bw / bh) : frame.w, sh = wide ? frame.h : frame.w / (bw / bh)
    c.drawImage(game, (frame.x + (frame.w - sw) / 2) * k, (frame.y + (frame.h - sh) / 2) * k, sw * k, sh * k, PAD, by, bw, bh)
  }
  c.restore()
  text('Serverway', PAD, 66, `600 40px ${NUM}`, v('ink'))
  if (tag) text(tag, W - PAD, 66, `800 28px ${KO}`, v('amber'), 'right')

  const x0 = tall ? PAD + bw + 32 : PAD, cols = tall ? 2 : 3, room = W - PAD - x0
  // 폭에 맞는 글자 크기
  const fit = (t: string, size: number, font: string) => { do { c.font = font.replace('$', String(size)) } while (c.measureText(t).width > room && --size > 20); return c.font }
  let y = tall ? by : by + bh + 32
  c.font = `800 30px ${KO}`
  box(x0, y, c.measureText(badge).width + 48, 58, 20, v(r.over === 'done' ? 'goodB' : 'badB'))
  text(badge, x0 + 24, y + 29, `800 30px ${KO}`, v(r.over === 'done' ? 'mint' : 'red'))
  y += 58 + 46
  for (const line of tall ? [`${clockText(r.seconds)} 동안 달려`, `${r.rank}까지`, '키웠습니다'] : [title]) { text(line, x0, y, fit(line, 46, `800 $px ${KO}`), v('ink')); y += 62 }
  y += 38
  const sw = text(score, x0, y, fit(`${score}00`, 96, `600 $px ${NUM}`), v('ink'))
  text('점', x0 + sw + 14, y + 18, `800 38px ${KO}`, v('sub'))
  const gap = 16, cw = (room - gap * (cols - 1)) / cols, ch = 124, rows = cells.length / cols
  const top = tall ? by + bh - rows * ch - (rows - 1) * gap : y + 78
  cells.forEach(([k, t], i) => {
    const x = x0 + (i % cols) * (cw + gap), cy = top + Math.floor(i / cols) * (ch + gap)
    box(x, cy, cw, ch, 26, v('card'))
    text(k, x + cw / 2, cy + 40, `400 27px ${KO}`, v('sub'), 'center')
    text(t, x + cw / 2, cy + 85, `600 32px ${NUM}`, v('ink'), 'center')
  })
  text(call, PAD, H - 58, `600 30px ${KO}`, v('periD'))
  text(site, W - PAD, H - 58, `400 27px ${KO}`, v('sub'), 'right')
  return new Promise((ok, no) => cv.toBlob((b) => (b ? ok(b) : no(new Error('blob'))), 'image/png'))
}

// 폰에서는 공유 창, PC에서는 복사. 둘 다 막힌 곳(앱 안의 브라우저)에서는 길게 눌러 저장하도록 그림 주소를 돌려준다.
export async function shareImage(r: Result, g: GameClient, frame: Frame | undefined): Promise<{ note: string; show?: string }> {
  const blob = await draw(r, g, frame)
  const file = new File([blob], `serverway-${r.score}.png`, { type: 'image/png' })
  if (matchMedia('(pointer: coarse)').matches) {
    if (navigator.canShare?.({ files: [file] })) try { await navigator.share({ files: [file] }); return { note: '' } } catch (e) { if ((e as Error).name === 'AbortError') return { note: '' } }
  } else try { await navigator.clipboard.write([new ClipboardItem({ 'image/png': blob })]); return { note: '이미지를 복사했습니다. 카톡이나 메신저에 붙여 넣으세요.' } } catch { /* 아래에서 그림을 보여 준다 */ }
  return { note: '', show: URL.createObjectURL(blob) }
}
