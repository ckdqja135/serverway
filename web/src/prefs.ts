// 화면 모드와 소리 설정. 고르기 전의 화면 모드는 기기 설정을 따른다 (index.html에서 먼저 적용한다).
export type Pref = 'dark' | 'bgm' | 'sfx'

const root = document.documentElement
export const getPref = (k: Pref) => (k === 'dark' ? root.dataset.theme === 'dark' : localStorage.getItem(`serverway.${k}`) !== 'off')

export function setPref(k: Pref, on: boolean) {
  if (k === 'dark') {
    root.dataset.theme = on ? 'dark' : 'light'
    localStorage.setItem('serverway.theme', root.dataset.theme)
  } else localStorage.setItem(`serverway.${k}`, on ? 'on' : 'off')
}
