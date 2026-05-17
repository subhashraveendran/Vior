import React, { useState } from 'react'
import { motion } from 'framer-motion'

const pageVariants = {
  initial: { opacity: 0, y: 20 },
  animate: { opacity: 1, y: 0, transition: { duration: 0.3, ease: 'easeOut' } },
  exit: { opacity: 0, y: -10, transition: { duration: 0.2 } }
}

export default function IdleView({ onStart }) {
  const [loading, setLoading] = useState(false)

  const handleClick = async () => {
    setLoading(true)
    try {
      await onStart()
    } catch {
      setLoading(false)
    }
  }

  return (
    <motion.div className="view" variants={pageVariants} initial="initial" animate="animate" exit="exit">
      <div className="idle-content">
        <motion.div
          className="idle-icon"
          initial={{ scale: 0.8, opacity: 0 }}
          animate={{ scale: 1, opacity: 0.3 }}
          transition={{ delay: 0.1, duration: 0.4 }}
        >
          <svg viewBox="0 0 24 24" width="56" height="56" fill="currentColor">
            <path d="M21 3H3c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h7v2H8v2h8v-2h-2v-2h7c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm0 14H3V5h18v12z"/>
          </svg>
        </motion.div>

        <motion.p
          className="idle-text"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.2 }}
        >
          Start the server to connect your phone as a second display
        </motion.p>

        <motion.button
          className="primary-btn"
          onClick={handleClick}
          disabled={loading}
          whileHover={{ scale: 1.03 }}
          whileTap={{ scale: 0.97 }}
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.3 }}
        >
          {loading ? (
            <span className="btn-loading">
              <span className="spinner-small" />
              Starting...
            </span>
          ) : 'Start Server'}
        </motion.button>
      </div>
    </motion.div>
  )
}
