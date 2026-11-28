import { useLayoutEffect, useRef, type MouseEvent } from 'react'

const SETTLE_MS = 350

// 저절로 뜬 창은 뜬 직후의 누름을 받지 않는다. 보드를 누르려던 손이나 Space가 창의 버튼을 대신 누르지 않게 한다.
// 키보드로 누른 것도 click으로 오므로 함께 막힌다. 창의 바깥 상자에 onClickCapture로 단다.
export function useSettle(open: boolean) {
  const at = useRef(0)
  useLayoutEffect(() => { if (open) at.current = performance.now() }, [open]) // 그리기 전에 재서 첫 화면의 누름도 막는다
  return (e: MouseEvent) => { if (performance.now() - at.current < SETTLE_MS) { e.preventDefault(); e.stopPropagation() } }
}
