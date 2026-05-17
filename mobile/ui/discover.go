package ui

import (
	"fmt"
	"image"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/subhashraveendran/vior/internal/discovery"
)

// DiscoverScreen shows discovered Vior servers on the LAN and allows manual IP entry.
type DiscoverScreen struct {
	th *material.Theme

	// Discovered servers.
	servers   []discovery.Beacon
	serversMu sync.Mutex
	scanning  bool

	// Manual connect.
	ipEditor   widget.Editor
	portEditor widget.Editor
	connectBtn widget.Clickable

	// Server list buttons (one per discovered server).
	serverBtns []widget.Clickable

	// Callback when user wants to connect.
	OnConnect func(host string, port int)

	// Scan control.
	scanStop chan struct{}
}

// NewDiscoverScreen creates a new discovery screen.
func NewDiscoverScreen(th *material.Theme) *DiscoverScreen {
	s := &DiscoverScreen{
		th: th,
	}
	s.ipEditor.SingleLine = true
	s.ipEditor.Submit = true
	s.portEditor.SingleLine = true
	s.portEditor.Submit = true
	return s
}

// StartScan begins scanning for Vior servers on the LAN.
func (s *DiscoverScreen) StartScan() {
	if s.scanning {
		return
	}
	s.scanning = true
	s.scanStop = make(chan struct{})

	go func() {
		for {
			select {
			case <-s.scanStop:
				return
			default:
			}

			beacons, err := discovery.Listen(3 * time.Second)
			if err != nil {
				continue
			}

			s.serversMu.Lock()
			s.servers = beacons
			// Ensure enough buttons.
			for len(s.serverBtns) < len(beacons) {
				s.serverBtns = append(s.serverBtns, widget.Clickable{})
			}
			s.serversMu.Unlock()
		}
	}()
}

// StopScan stops the scan.
func (s *DiscoverScreen) StopScan() {
	if !s.scanning {
		return
	}
	s.scanning = false
	close(s.scanStop)
}

// Layout draws the discover screen.
func (s *DiscoverScreen) Layout(gtx layout.Context) layout.Dimensions {
	// Check manual connect button.
	if s.connectBtn.Clicked(gtx) {
		s.handleConnect()
	}

	// Check server list clicks.
	s.serversMu.Lock()
	servers := make([]discovery.Beacon, len(s.servers))
	copy(servers, s.servers)
	s.serversMu.Unlock()

	for i := range servers {
		if i < len(s.serverBtns) && s.serverBtns[i].Clicked(gtx) {
			if s.OnConnect != nil {
				s.OnConnect(servers[i].Name, servers[i].Port)
			}
		}
	}

	// Fill background.
	paint.FillShape(gtx.Ops, ColorBg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Title.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top: unit.Dp(60), Left: unit.Dp(24), Right: unit.Dp(24), Bottom: unit.Dp(8),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return H1(s.th, "Vior").Layout(gtx)
			})
		}),
		// Subtitle.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Left: unit.Dp(24), Right: unit.Dp(24), Bottom: unit.Dp(24),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return Caption(s.th, "Connect to your computer").Layout(gtx)
			})
		}),
		// Discovered servers list.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutServers(gtx, servers)
		}),
		// Manual connect section.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutManualConnect(gtx)
		}),
	)
}

func (s *DiscoverScreen) layoutServers(gtx layout.Context, servers []discovery.Beacon) layout.Dimensions {
	if len(servers) == 0 {
		return layout.Inset{
			Left: unit.Dp(24), Right: unit.Dp(24), Bottom: unit.Dp(16),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return Caption(s.th, "Scanning for servers...").Layout(gtx)
		})
	}

	items := make([]layout.FlexChild, 0, len(servers)+1)
	items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{
			Left: unit.Dp(24), Right: unit.Dp(24), Bottom: unit.Dp(8),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return Caption(s.th, "Found servers:").Layout(gtx)
		})
	}))

	for i, srv := range servers {
		i, srv := i, srv
		items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Left: unit.Dp(24), Right: unit.Dp(24), Bottom: unit.Dp(8),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutServerItem(gtx, i, srv)
			})
		}))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
}

func (s *DiscoverScreen) layoutServerItem(gtx layout.Context, idx int, srv discovery.Beacon) layout.Dimensions {
	btn := material.Button(s.th, &s.serverBtns[idx], fmt.Sprintf("%s (%s)", srv.Name, srv.Platform))
	btn.Background = ColorSurface
	btn.Color = ColorText
	btn.CornerRadius = unit.Dp(10)
	return btn.Layout(gtx)
}

func (s *DiscoverScreen) layoutManualConnect(gtx layout.Context) layout.Dimensions {
	return layout.Inset{
		Top: unit.Dp(16), Left: unit.Dp(24), Right: unit.Dp(24),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return Caption(s.th, "Or enter IP address:").Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Flexed(0.65, func(gtx layout.Context) layout.Dimensions {
						ed := material.Editor(s.th, &s.ipEditor, "IP Address")
						ed.Color = ColorText
						ed.HintColor = ColorTextDim
						ed.TextSize = unit.Sp(16)
						return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							// Background.
							paint.FillShape(gtx.Ops, ColorSurface, clip.RRect{
								Rect: image.Rectangle{Max: gtx.Constraints.Max},
								NE:   8, NW: 8, SE: 8, SW: 8,
							}.Op(gtx.Ops))
							return layout.Inset{
								Top: unit.Dp(10), Bottom: unit.Dp(10),
								Left: unit.Dp(12), Right: unit.Dp(12),
							}.Layout(gtx, ed.Layout)
						})
					}),
					layout.Flexed(0.35, func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(s.th, &s.connectBtn, "Connect")
						btn.Background = ColorPrimary
						btn.Color = ColorWhite
						btn.CornerRadius = unit.Dp(10)
						return btn.Layout(gtx)
					}),
				)
			}),
		)
	})
}

func (s *DiscoverScreen) handleConnect() {
	ip := s.ipEditor.Text()
	if ip == "" {
		return
	}
	port := 8080
	if s.OnConnect != nil {
		s.OnConnect(ip, port)
	}
}
