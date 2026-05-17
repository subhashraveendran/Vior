import React, { useEffect } from 'react'
import { motion } from 'framer-motion'

const typeColors = {
  info: '#6366f1',
  success: '#34d399',
  error: '#ef4444'
}

export default function Toast({ message, type = 'info', onDone }) {
  useEffect(() => {
    const timer = setTimeout(onDone, 2500)
    return () => clearTimeout(timer)
  }, [onDone])

  return (
    <motion.div
      className="toast"
      style={{ borderColor: typeColors[type] }}
      initial={{ opacity: 0, y: 30, scale: 0.95 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      exit={{ opacity: 0, y: 10, scale: 0.95 }}
      transition={{ duration: 0.2 }}
    >
      <span
        className="toast-dot"
        style={{ background: typeColors[type] }}
      />
      {message}
    </motion.div>
  )
}
