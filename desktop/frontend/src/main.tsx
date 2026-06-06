import React from 'react'
import { createRoot } from 'react-dom/client'
import App from './components/App'
import './style.css'
import './app.css'

const root = document.getElementById('app')
if (!root) throw new Error('#app root element not found')
createRoot(root).render(<App />)
