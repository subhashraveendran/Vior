# 🚀 Vior – Complete Project Planld a cross-platform system that:
- Streams screen (low latency)
- Supports extended display (via real/dummy display)
- Provides input control (mouse/keyboard)
- Enables file transfer
- Works as:
  - CLI tool (core engine)
  - Desktop app (UI wrapper)
  - Mobile/web client

---

# 🧠 Core Architecture

## Layered Design

1. CLI Core (Go)
2. Desktop App (Wails + Vite)
3. Client (Mobile/Web)

```
[CLI Core (Go)]
   ├─ Capture
   ├─ Stream
   ├─ Input
   ├─ File Transfer
   └─ Networking

        ↓

[Wails App]
   ├─ UI
   ├─ Controls
   └─ Settings

        ↓

[Client]
   ├─ Phone / Tablet
   └─ Browser/App
```

---

# ⚠️ Key Constraint (Important)

- True virtual display driver on macOS → ❌ Not feasible
- Solution → Use:
  - Dummy HDMI
  - OR BetterDummy

👉 This creates a REAL extended display

---

# 🛠️ Phase-by-Phase Plan

---

## 🥇 Phase 1 – MVP (Day 1–2)

### Goal:
Stream screen to another device

### Tasks:
- Setup Go project
- Capture screen
- Serve over HTTP (MJPEG)

### Output:
- Open URL on phone → see screen

---

## 🥈 Phase 2 – CLI Structure

### Tool:
- Cobra (Go CLI framework)

### Commands:
```
vior start
screeviorw stop
vioreatureviordisplays
- Select display index

---

## 🥉 Phase 3 – Extend Display Support

### Setup:
- Plug dummy HDMI OR use BetterDummy

### Logic:
- Detect displays
- Capture selected display

```
Display 0 → main
Display 1 → extended (important)
```

---

## 🏗️ Phase 4 – File Transfer

### Features:
- Send files
- Receive files
- Progress tracking

### Implementation:
- TCP or HTTP endpoints

---

## 🎮 Phase 5 – Input Control

### Tool:
- robotgo

### Features:
- Mouse move
- Click
- Keyboard input

---

## 🌐 Phase 6 – Networking

### Start with:
- Manual IP connection

### Then:
- QR code connection
- Optional mDNS discovery

---

## ⚡ Phase 7 – Performance Upgrade

### Replace MJPEG with:
- WebRTC OR
- H264 (FFmpeg / VideoToolbox)

### Goals:
- Lower latency
- Better FPS

---

## 🔌 Phase 8 – USB Support (Android)

### Tool:
- ADB port forwarding

### Benefit:
- Wired low-latency connection

---

## 🖥️ Phase 9 – Desktop App (Wails)

### Stack:
- Wails + Vite

### UI Features:
- Select display
- Connect device
- Start/stop stream
- File transfer UI

---

# 📁 Project Structure

``ow/
 ├─ cmd/
 ├─ internal/
 │   ├─vior │   ├─ stream/
 │   ├─ transfer/
 │   ├─ input/
 │   └─ network/
 ├─ main.go
```

---

# 🔥 Final Product Features

- Screen streaming (multi-display)
- Extend display (via real display)
- Input control
- File transfer
- Cross-platform CLI
- Desktop app UI
- Mobile/browser client

---

# 🚫 What NOT to do

- Don’t build macOS display driver
- Don’t start with UI
- Don’t jump to WebRTC early

---

# 🧠 Execution Strategy

1. Make streaming work
2. Add CLI commands
3. Add multi-display support
4. Add file transfer
5. Add input control
6. Build UI
7. Optimize performance

---

# 💡 End Vision

A system like:

> “Open-s

# 🚀 Next Step

👉 Run MVP streamiVior — Extend your view. Stream, control, transfer.---

(You now have a complete roadmap — follow sequentially, don’t skip phases)

