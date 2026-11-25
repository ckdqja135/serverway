import { useCallback, useEffect, useLayoutEffect, useRef } from 'react'

// 내용이 화면보다 길면 스크롤이 생기지 않게 그만큼 줄여서 보여 준다.
// 글자 크기를 키운 폰이나 주소창이 있는 앱 안 브라우저처럼 높이를 미리 알 수 없는 곳을 위한 것이다.
export function useFit() {
  const ref = useRef<HTMLDivElement>(null)
  const fit = useCallback(() => {
    const el = ref.current, box = el?.parentElement
    if (!el || !box) return
    el.style.zoom = '1'
    const s = getComputedStyle(box), room = box.clientHeight - parseFloat(s.paddingTop) - parseFloat(s.paddingBottom)
    el.style.zoom = String(Math.max(0.55, Math.min(1, (room - 1) / el.offsetHeight)))
  }, [])
  useLayoutEffect(fit)
  useEffect(() => {
    document.fonts.ready.then(fit)
    window.addEventListener('resize', fit)
    return () => window.removeEventListener('resize', fit)
  }, [])
  return ref
}
