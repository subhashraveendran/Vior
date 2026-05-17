import React, { useState, useEffect, useCallback } from 'react'
import { motion } from 'framer-motion'
import { GetUSBStatus, SetupUSB, TeardownUSB, ListDisplays, GetVersion, DownloadADB } from '../../wailsjs/go/main/App'
import { QualityCard, Toggle } from '../design/Primitives'
import { Icons } from '../design/Icons'
import { T } from '../design/tokens'

const PRESETS = [
  { name: 'Low', sub: '480p · 30 fps', q: 40, fps: 15 },
  { name: 'Medium', sub: '720p · 30 fps', q: 65, fps: 24 },
  { name: 'High', sub: '1080p · 60 fps', q: 80, fps: 30 },
  { name: 'Ultra', sub: 'Native · 60 fps', q: 95, fps: 60 },
]

export default function Settings({ config, onChange, onClose }) {
  const [activePreset, setActivePreset] = useState(2)
  const [usb, setUsb] = useState({ available: false, connected: false, forwarding: false })
  const [displays, setDisplays] = useState([])
  const [version, setVersion] = useState('')
  const [usbLoading, setUsbLoading] = useState(false)

  useEffect(() => {
    const idx = PRESETS.findIndex(p => p.q === config.quality && p.fps === config.frameRate)
    if (idx >= 0) setActivePreset(idx)
  }, [config])

  useEffect(() => {
    (async () => {
      try { setUsb(await GetUSBStatus()) } catch {}
      try { setDisplays(await ListDisplays()) } catch {}
      try { setVersion(await GetVersion()) } catch {}
    })()
    const id = setInterval(async () => { try { setUsb(await GetUSBStatus()) } catch {} }, 5000)
    return () => clearInterval(id)
  }, [])

  const selectPreset = useCallback((idx) => {
    setActivePreset(idx)
    const p = PRESETS[idx]
    onChange({ port: config.port, quality: p.q, frameRate: p.fps })
  }, [config.port, onChange])

  const handleUSB = async () => {
    setUsbLoading(true)
    try {
      if (usb.forwarding) await TeardownUSB()
      else await SetupUSB()
      setUsb(await GetUSBStatus())
    } catch (e) { alert(String(e)) }
    setUsbLoading(false)
  }

  const handleDownloadADB = async () => {
    setUsbLoading(true)
    try { await DownloadADB(); setUsb(await GetUSBStatus()) } catch (e) { alert('Download failed: ' + e) }
    setUsbLoading(false)
  }

  return (
    <>
      <motion.div className="settings-backdrop"
        initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
        onClick={onClose}/>

      <motion.div className="settings-panel"
        initial={{ x: 380 }} animate={{ x: 0 }} exit={{ x: 380 }}
        transition={{ type: 'spring', stiffness: 300, damping: 30 }}>

        <div className="settings-header">
          <span className="settings-title">Settings</span>
          <button className="icon-btn" onClick={onClose}>{Icons.x(16)}</button>
        </div>

        <div className="settings-scroll">
          {/* Quality */}
          <div>
            <div className="section-label">Quality</div>
            <div className="quality-grid">
              {PRESETS.map((p, i) => (
                <QualityCard key={p.name} name={p.name} sub={p.sub} active={activePreset === i} onClick={() => selectPreset(i)}/>
              ))}
            </div>
          </div>

          {/* USB */}
          <div>
            <div className="section-label">USB</div>
            <div style={{ padding: 14, borderRadius: 10, background: T.surface2, boxShadow: `inset 0 0 0 1px ${T.border}` }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 8 }}>
                <span style={{
                  width: 8, height: 8, borderRadius: '50%',
                  background: usb.available ? T.success : T.textDim,
                  boxShadow: usb.available ? '0 0 0 3px rgba(52,211,153,0.18)' : 'none',
                }}/>
                <span style={{ fontSize: 12.5, color: T.heading, fontWeight: 500 }}>
                  {usb.available ? (usb.connected ? usb.deviceName || 'Device connected' : 'No device') : 'ADB not installed'}
                </span>
                <span style={{ flex: 1 }}/>
                {usb.available && usb.connected && <Toggle on={usb.forwarding} onChange={handleUSB}/>}
              </div>
              <div style={{ fontSize: 11.5, color: T.textDim, lineHeight: 1.5 }}>
                {usb.available
                  ? 'USB connection works without Wi-Fi. Plug in your Android device.'
                  : 'Required for USB connection. Downloads ~5 MB from Google.'}
              </div>
              {!usb.available && (
                <button onClick={handleDownloadADB} disabled={usbLoading} style={{
                  marginTop: 10, appearance: 'none', border: 0, cursor: 'pointer',
                  background: 'transparent', color: T.indigo2,
                  fontSize: 12, fontWeight: 500, padding: 0, fontFamily: 'inherit',
                }}>
                  {usbLoading ? 'Downloading…' : 'Install ADB →'}
                </button>
              )}
            </div>
          </div>

          {/* Displays */}
          <div>
            <div className="section-label">Displays</div>
            {displays.map(d => (
              <div key={d.index} className="display-row">
                {Icons.monitor(16, { color: T.textDim })}
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 12.5, color: T.heading, fontWeight: 500 }}>{d.name}</div>
                  <div className="mono" style={{ fontSize: 10.5, color: T.textDim, marginTop: 2 }}>{d.width} × {d.height}</div>
                </div>
                {d.isMain && <span className="primary-badge">Primary</span>}
              </div>
            ))}
          </div>

          {/* About */}
          <div>
            <div className="section-label">About</div>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '2px 0' }}>
              <span style={{ fontSize: 12.5, color: T.text }}>Vior</span>
              <span className="mono" style={{ fontSize: 11.5, color: T.textDim }}>{version}</span>
            </div>
          </div>
        </div>
      </motion.div>
    </>
  )
}
