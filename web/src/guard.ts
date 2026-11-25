// 우클릭 메뉴와 개발자 도구 단축키를 막는다. 브라우저 메뉴로 여는 것까지는 막을 수 없다.
const blocked = (e: KeyboardEvent) => {
  // 한글 입력 상태에서도 같은 자리의 글쇠로 알아보도록 key 대신 code를 쓴다.
  const mod = e.ctrlKey || e.metaKey
  if (e.code === 'F12') return true
  if (mod && (e.shiftKey || e.altKey) && ['KeyI', 'KeyJ', 'KeyC', 'KeyK'].includes(e.code)) return true
  return mod && (e.code === 'KeyU' || (e.code === 'KeyS' && !e.shiftKey)) // 소스 보기, 페이지 저장
}

export function guard() {
  window.addEventListener('contextmenu', (e) => { if (!(e.target instanceof HTMLImageElement)) e.preventDefault() }, true) // 기록 이미지는 길게 눌러 저장할 수 있다
  window.addEventListener('keydown', (e) => { if (blocked(e)) { e.preventDefault(); e.stopPropagation() } }, true)
}
