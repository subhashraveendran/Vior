# Vior - 2-Month Detailed Sprint Plan

## Overview
This plan breaks down the implementation of Vior into detailed 2-week sprints with specific, achievable goals. The plan focuses on building a working MVP first, then incrementally adding features based on the project requirements.

## Month 1

### Week 1: Foundation & MVP Streaming
**Goal**: Basic screen streaming capability

**Sprint Focus**: Core streaming implementation
- Set up Go project structure
- Implement basic screen capture for main display
- Create simple HTTP MJPEG streaming server
- Test streaming with a basic web client

**Deliverable**: 
- Basic streaming from primary display viewable in a browser
- Working Go project with modular structure
- Basic CLI with start/stop commands

**What can be done**:
- Set up project structure
- Implement screen capture for main display
- Create HTTP streaming server
- Basic CLI interface

**What cannot be done**:
- Full feature set implementation
- Cross-platform support (will need more time)
- Performance optimization (H264/WebRTC)
- Input control or file transfer

### Week 2: CLI Enhancement & Multi-display Support
**Goal**: Enhanced CLI with multi-display support

**Sprint Focus**: CLI structure and multi-display capability
- Implement Cobra CLI structure
- Add display enumeration
- Add display selection functionality
- Basic error handling and configuration

**Deliverable**:
- Fully functional CLI with all planned commands
- Multi-display enumeration and selection
- Basic configuration system

**What can be done**:
- Complete CLI with all planned commands
- Display enumeration and selection
- Basic configuration system

**What cannot be done**:
- Full cross-platform support
- Advanced display management
- Performance optimization

### Week 3: File Transfer Implementation
**Goal**: Implement file transfer capabilities

**Sprint Focus**: Implement TCP-based file transfer
- Add progress tracking
- Create simple UI for file transfer

**Deliverable**:
- Basic file transfer capability
- Progress tracking for transfers
- Simple UI for testing

**What can be done**:
- Basic file transfer with progress tracking
- Simple UI for testing

**What cannot be done**:
- P2P optimized file transfer
- Advanced file management
- Cross-platform file system handling

### Week 4: Input Control & Networking
**Goal**: Add input control and networking features

**Sprint Focus**: Input control and networking
- Implement mouse/keyboard control using robotgo
- Add networking features (manual IP, QR code)
- Basic connection management

**Deliverable**:
- Basic mouse and keyboard control
- Manual IP connection capability
- QR code connection setup

**What can be done**:
- Basic input control implementation
- Manual IP connections
- QR code setup

**What cannot be done**:
- Cross-platform input handling
- Advanced networking protocols
- P2P connections

## Month 2

### Week 5: Performance Optimization & WebRTC Implementation
**Goal**: Improve performance and implement WebRTC

**Sprint Focus**: Performance improvements
- Replace MJPEG with WebRTC
- Optimize frame rate and latency
- Implement proper error handling

**Deliverable**:
- WebRTC implementation
- Improved performance and reduced latency
- Better error handling

**What can be done**:
- WebRTC streaming implementation
- Performance optimizations
- Better error handling

**What cannot be done**:
- Perfect cross-platform compatibility
- All possible optimizations
- Advanced features like USB support

### Week 6: Desktop App & UI Implementation
**Goal**: Create desktop application with UI

**Sprint Focus**: Desktop application development
- Implement Wails + Vite desktop app
- Create UI for display selection
- Add UI controls for streaming

**Deliverable**:
- Basic desktop application with UI
- Display selection interface
- Basic connection management UI

**What can be done**:
- Basic UI implementation
- Display selection
- Start/stop controls

**What cannot be done**:
- Fully polished UI
- Advanced settings
- Cross-platform UI consistency

### Week 7: Mobile/Web Client Development
**Goal**: Create mobile/web client

**Sprint Focus**: Client application development
- Create web-based client interface
- Implement mobile-friendly controls
- Add connection management

**Deliverable**:
- Basic web client interface
- Simple connection management
- Basic controls for streaming

**What can be done**:
- Basic web interface
- Simple connection management
- Basic controls

**What cannot be done**:
- Native mobile applications
- Advanced mobile features
- Full cross-platform support

### Week 8: Testing, Documentation & Polish
**Goal**: Testing and finalization

**Sprint Focus**: Quality assurance and documentation
- Comprehensive testing across platforms
- Create documentation
- Polish user experience

**Deliverable**:
- Tested application across platforms
- Basic documentation
- Polished user experience

**What can be done**:
- Basic testing
- Documentation
- UX improvements

**What cannot be done**:
- Comprehensive cross-platform testing
- Full documentation
- Production-ready polish

## Key Limitations & Constraints

1. **Technical Limitations**:
   - Cannot create virtual display drivers for macOS (as noted in constraints)
   - Must use physical dummy displays or BetterDummy
   - Limited cross-platform support in 2 months

2. **Time Constraints**:
   - Cannot implement all features perfectly
   - Performance optimization will be limited
   - Advanced features like USB support will be basic implementation

3. **Feature Limitations**:
   - Input control will be basic
   - File transfer will be simple implementation
   - Networking will be manual IP-based

This plan provides a realistic roadmap for implementing Vior over 2 months, focusing on achievable goals while acknowledging the limitations of the timeframe.