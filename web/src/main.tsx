import '@fontsource/gothic-a1/400.css'
import '@fontsource/gothic-a1/600.css'
import '@fontsource/gothic-a1/800.css'
import '@fontsource/unbounded/600.css'
import './styles.css'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { guard } from './guard'
import { startSound } from './sound'
import { startWatch } from './watch'

if (import.meta.env.PROD) guard() // 개발 서버에서는 막지 않는다
startSound()
startWatch()
createRoot(document.getElementById('root')!).render(<App />)
