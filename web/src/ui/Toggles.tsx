import { Moon, Music, Sun, Volume2, VolumeX } from 'lucide-react'
import { useReducer } from 'react'
import { getPref, setPref, type Pref } from '../prefs'
import { applySound, sfx } from '../sound'

export function Toggles() {
  const [, redraw] = useReducer((n: number) => n + 1, 0)
  const flip = (k: Pref) => {
    setPref(k, !getPref(k))
    if (k === 'bgm') applySound()
    if (k === 'sfx') sfx('link')
    redraw()
  }
  const dark = getPref('dark'), bgm = getPref('bgm'), fx = getPref('sfx')
  return (
    <div className="toggles">
      <button onClick={() => flip('dark')}>{dark ? <Moon size={16} aria-hidden /> : <Sun size={16} aria-hidden />}{dark ? '다크' : '라이트'}</button>
      <button className={bgm ? '' : 'off'} onClick={() => flip('bgm')}><Music size={16} aria-hidden />BGM {bgm ? 'ON' : 'OFF'}</button>
      <button className={fx ? '' : 'off'} onClick={() => flip('sfx')}>{fx ? <Volume2 size={16} aria-hidden /> : <VolumeX size={16} aria-hidden />}효과음 {fx ? 'ON' : 'OFF'}</button>
    </div>
  )
}
