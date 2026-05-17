import React, { useState, useEffect, useCallback } from 'react'
import { motion } from 'framer-motion'
import {
  GetUSBStatus, SetupUSB, TeardownUSB,
  ListDisplays, GetVersion, DownloadADB
} from '../../wailsjs/go/main/App'

const PRESETS = [
  { label: 'Low', quality: 40, fps: 15, desc: 'Save bandwidth' },
  { label: 'Medium', quality: 65, fps: 24, desc: 'Balanced' },
  { label: 'High', quality: 80, fps: 30, desc: 'Recommended' },
  { label: 'Ultra', quality: 95, fps: 60, desc: 'Best quality' },
]

export default function Settings({ config, onChange, onClose }) {
  const [activePreset, setActivePreset] = useState(2) // High default
  const [usb, setUsb] = useState({ available: false, connected: false, forwarding: false, deviceName: '' })
  const [displays, setDisplays] = useState([])
  const [version, setVersion] = useState('')
  const [usbLoading, setUsbLoading] = useState(false)

  // Match current config to a preset.
  useEffect(() => {
    const idx = PRESETS.findIndex(p => p.quality === config.quality && p.fps === config.frameRate)
    if (idx >= 0) setActivePreset(idx)
  }, [config])

  useEffect(() => {
    (async () => {
      try { setUsb(await GetUSBStatus()) } catch {}
      try { setDisplays(await ListDisplays()) } catch {}
      try { setVersion(await GetVersion()) } catch {}
    })()
    const id = setInterval(async () => {
      try { setUsb(await GetUSBStatus()) } catch {}
    }, 5000)
    return () => clearInterval(id)
  }, [])

  const selectPreset = useCallback((idx) => {
    setActivePreset(idx)
    const p = PRESETS[idx]
    onChange({ port: config.port, quality: p.quality, frameRate: p.fps })
  }, [config.port, onChange])

  const handleUSBToggle = async () => {
    setUsbLoading(true)
    try {
      if (usb.forwarding) {
        await TeardownUSB()
      } else {
        await SetupUSB()
      }
      setUsb(await GetUSBStatus())
    } catch (e) {
      alert(String(e))
    }
    setUsbLoading(false)
  }

  const handleDownloadADB = async () => {
    setUsbLoading(true)
    try {
      await DownloadADB()
      setUsb(await GetUSBStatus())
    } catch (e) {
      alert('Download failed: ' + e)
    }
    setUsbLoading(false)
  }

  return (
    <>
      <motion.div
        className="settings-backdrop"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        onClick={onClose}
      />
      <motion.div
        className="settings"
        initial={{ x: 340 }}
        animate={{ x: 0 }}
        exit={{ x: 340 }}
        transition={{ type: 'spring', stiffness: 300, damping: 30 }}
      >
        <div className="settings-header">
          <h2>Settings</h2>
          <button className="close-btn" onClick={onClose}>
            <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
              <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/>
            </svg>
          </button>
        </div>

        <div className="settings-scroll">
          {/* ── Quality Presets ────────────────────────── */}
          <section className="settings-section">
            <h3 className="section-label">Quality</h3>
            <div className="preset-grid">
              {PRESETS.map((p, i) => (
                <button
                  key={p.label}
                  className={`preset-card ${activePreset === i ? 'active' : ''}`}
                  onClick={() => selectPreset(i)}
                >
                  <span className="preset-name">{p.label}</span>
                  <span className="preset-desc">{p.desc}</span>
                </button>
              ))}
            </div>
          </section>

          {/* ── USB Connection ─────────────────────────── */}
          <section className="settings-section">
            <h3 className="section-label">USB Connection</h3>
            <div className="conn-card">
              <div className="conn-row">
                <div className="conn-icon">
                  <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">
                    <path d="M15 7v4h1v2h-3V5h2l-3-4-3 4h2v8H8v-2.07A1.993 1.993 0 006 5a2 2 0 00-1 3.75V11c0 1.1.9 2 2 2h3v3.05A1.993 1.993 0 0012 21a2 2 0 001-3.75V13h3c1.1 0 2-.9 2-2V9h1V7h-4z"/>
                  </svg>
                </div>
                <div className="conn-info">
                  <span className="conn-title">
                    {!usb.available
                      ? 'ADB Not Installed'
                      : !usb.connected
                        ? 'No Device'
                        : usb.deviceName || 'Android Device'}
                  </span>
                  <span className="conn-sub">
                    {!usb.available
                      ? 'Required for USB connection'
                      : !usb.connected
                        ? 'Connect Android via USB cable'
                        : usb.forwarding
                          ? 'Port forwarding active'
                          : 'Ready to connect'}
                  </span>
                </div>
                {usb.available && usb.connected && (
                  <button
                    className={`pill-toggle ${usb.forwarding ? 'on' : ''}`}
                    onClick={handleUSBToggle}
                    disabled={usbLoading}
                  >
                    <span className="pill-knob" />
                  </button>
                )}
              </div>
              {!usb.available && (
                <button
                  className="action-btn"
                  onClick={handleDownloadADB}
                  disabled={usbLoading}
                >
                  {usbLoading ? 'Downloading...' : 'Install ADB'}
                </button>
              )}
            </div>
          </section>

          {/* ── Displays ───────────────────────────────── */}
          <section className="settings-section">
            <h3 className="section-label">Displays</h3>
            <div className="display-list">
              {displays.map((d) => (
                <div className="disp-row" key={d.index}>
                  <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor" opacity="0.4">
                    <path d="M21 3H3c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h7v2H8v2h8v-2h-2v-2h7c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm0 14H3V5h18v12z"/>
                  </svg>
                  <span className="disp-name">{d.name}</span>
                  {d.isMain && <span className="disp-badge">Primary</span>}
                  <span className="disp-res">{d.width}×{d.height}</span>
                </div>
              ))}
            </div>
          </section>

          {/* ── About ──────────────────────────────────── */}
          <section className="settings-section about-section">
            <div className="about-item">
              <span>Vior</span>
              <span className="about-val">{version}</span>
            </div>
          </section>
        </div>
      </motion.div>
    </>
  )
}
