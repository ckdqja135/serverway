// 게임 규칙(Go)을 화면과 떨어진 워커에서 돌린다.
import './wasm/wasm_exec.js'
import wasm from './wasm/game.wasm?url'

declare const Go: new () => { importObject: WebAssembly.Imports; run(i: WebAssembly.Instance): Promise<void> }
const scope = self as unknown as { serverway(m: string): void; postMessage(m: string): void; onmessage: (e: MessageEvent<string>) => void }

const go = new Go()
const ready = fetch(wasm)
  .then((r) => { if (!r.ok) throw new Error(String(r.status)); return r.arrayBuffer() })
  .then((b) => WebAssembly.instantiate(b, go.importObject))
  .then((r) => { go.run(r.instance) })
ready.catch(() => scope.postMessage('{"type":"fail"}'))
scope.onmessage = (e) => { ready.then(() => scope.serverway(e.data), () => {}) }
