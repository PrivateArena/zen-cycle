package ui

import (
	"fmt"
	"image/color"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"zen-cycle/pkg/config"
	"zen-cycle/pkg/link"
)

func RunUI(window *app.Window, cfg *config.Config) error {
	th := material.NewTheme()
	th.Palette.Bg = colorBg
	th.Palette.Fg = colorText
	th.Palette.ContrastBg = colorAccent
	th.Palette.ContrastFg = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

	var uiState UIState
	uiState.ActiveProjectIndex.Store(-1)
	uiState.SymlinkInput.SetText("profile")
	uiState.ProfileList.Axis = layout.Vertical

	// Create event channels for background workers
	eventChan := make(chan BackgroundEvent, 20)
	triggerScanChan := make(chan int, 20)

	scanTicker := time.NewTicker(2 * time.Second)
	defer scanTicker.Stop()

	// Run background scan coordinator
	go func() {
		for {
			select {
			case idx := <-triggerScanChan:
				config.ConfigMu.Lock()
				if idx < 0 || idx >= len(cfg.Projects) {
					config.ConfigMu.Unlock()
					continue
				}
				p := cfg.Projects[idx]
				config.ConfigMu.Unlock()

				// Perform checks
				sources, err := link.GetCycleSources(p.Path)
				if err != nil {
					eventChan <- BackgroundEvent{Type: "ERROR", Message: fmt.Sprintf("Failed to list sources: %v", err)}
					window.Invalidate()
					continue
				}

				active, err := link.DetectActiveSource(p.Path, p.SymlinkName)
				if err != nil && err != link.ErrRealDirectory {
					eventChan <- BackgroundEvent{Type: "ERROR", Message: fmt.Sprintf("Failed checking active link: %v", err)}
				}

				msg := ""
				if err == link.ErrRealDirectory {
					msg = "WARNING: Target path is a real directory, not a symlink! Overwriting is blocked."
				}

				eventChan <- BackgroundEvent{
					Type:          "SCAN_RESULT",
					Profiles:      sources,
					ActiveProfile: active,
					Message:       msg,
				}
				window.Invalidate()

			case <-scanTicker.C:
				// Periodically refresh the selected project
				activeIdx := uiState.ActiveProjectIndex.Load()
				if activeIdx >= 0 {
					triggerScanChan <- int(activeIdx)
				}
			}
		}
	}()

	var ops op.Ops

	for {
		ev := window.Event()
		switch e := ev.(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			// Read all pending events from worker thread
			for {
				select {
				case bEv := <-eventChan:
					switch bEv.Type {
					case "SCAN_RESULT":
						uiState.AvailableProfiles = bEv.Profiles
						uiState.DetectedActive = bEv.ActiveProfile
						if bEv.Message != "" {
							uiState.ActiveError = bEv.Message
						} else {
							uiState.ActiveError = ""
						}
						// Resize buttons matching available profiles list
						if len(uiState.SwitchButtons) != len(bEv.Profiles) {
							uiState.SwitchButtons = make([]widget.Clickable, len(bEv.Profiles))
						}
					case "SWITCH_DONE":
						config.ConfigMu.Lock()
						if bEv.Index >= 0 && bEv.Index < len(cfg.Projects) {
							cfg.Projects[bEv.Index].CurrentActive = bEv.ActiveProfile
						}
						config.ConfigMu.Unlock()
						_ = config.SaveConfigAtomic(cfg)
						uiState.ActiveSuccess = bEv.Message
						uiState.ActiveError = ""
						uiState.Toasts.Push(bEv.Message, ToastSuccess, ToastDefaultDuration)
					case "ERROR":
						uiState.ActiveError = bEv.Message
						uiState.ActiveSuccess = ""
						uiState.Toasts.Push(bEv.Message, ToastError, ToastDefaultDuration)
					case "SUCCESS":
						uiState.ActiveSuccess = bEv.Message
						uiState.ActiveError = ""
						uiState.Toasts.Push(bEv.Message, ToastSuccess, ToastDefaultDuration)
					}
				default:
					break
				}
			if len(eventChan) == 0 {
				break
			}
		}

		// Drain expired toasts and clear corresponding persistent state
		for _, expired := range uiState.Toasts.DrainExpired() {
			switch expired.Type {
			case ToastError:
				if uiState.ActiveError == expired.Message {
					uiState.ActiveError = ""
				}
			case ToastSuccess:
				if uiState.ActiveSuccess == expired.Message {
					uiState.ActiveSuccess = ""
				}
			}
		}

		gtx := app.NewContext(&ops, e)

			// Process button clicks and user inputs
			handleInputs(gtx, &uiState, cfg, triggerScanChan, eventChan, window)

			// Perform component layouts
			layoutMain(gtx, th, &uiState, cfg)

			e.Frame(gtx.Ops)
		}
	}
}
