package main

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// UIState captures the localized visual state of the application.
type UIState struct {
	ActiveProjectIndex atomic.Int32
	AvailableProfiles  []string
	DetectedActive     string
	ActiveError        string
	ActiveSuccess      string
	
	// Editors for Adding Projects
	NameInput     widget.Editor
	PathInput     widget.Editor
	SymlinkInput  widget.Editor
	DenylistInput widget.Editor

	// Buttons
	AddProjectBtn  widget.Clickable
	RefreshBtn     widget.Clickable
	DeleteBtn      widget.Clickable
	SwitchButtons  []widget.Clickable
	ProjectButtons []widget.Clickable
}

// BackgroundEvent represents async updates from worker to UI thread.
type BackgroundEvent struct {
	Type          string // "SCAN_RESULT", "ERROR", "SUCCESS"
	Profiles      []string
	ActiveProfile string
	Message       string
}

// Custom industrial/minimalist color palette
var (
	colorBg      = color.NRGBA{R: 0x12, G: 0x12, B: 0x12, A: 0xff} // Main Dark BG
	colorSidebar = color.NRGBA{R: 0x1e, G: 0x1e, B: 0x1e, A: 0xff} // Sidebar/Card BG
	colorAccent  = color.NRGBA{R: 0xff, G: 0x6b, B: 0x00, A: 0xff} // Industrial Amber Accent
	colorText    = color.NRGBA{R: 0xe0, G: 0xe0, B: 0xe0, A: 0xff} // Primary Text
	colorSubtext = color.NRGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff} // Muted Subtext
	colorGreen   = color.NRGBA{R: 0x33, G: 0xcc, B: 0x33, A: 0xff} // Success Green
	colorRed     = color.NRGBA{R: 0xff, G: 0x33, B: 0x33, A: 0xff} // Danger/Error Red
)

func runUI(window *app.Window, cfg *Config) error {
	th := material.NewTheme()
	th.Palette.Bg = colorBg
	th.Palette.Fg = colorText
	th.Palette.ContrastBg = colorAccent
	th.Palette.ContrastFg = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

	var uiState UIState
	uiState.ActiveProjectIndex.Store(-1)
	uiState.SymlinkInput.SetText("profile")

	// Create event channels for background workers
	eventChan := make(chan BackgroundEvent, 20)
	triggerScanChan := make(chan int, 20)

	// Run background scan coordinator
	go func() {
		for {
			select {
			case idx := <-triggerScanChan:
				configMu.Lock()
				if idx < 0 || idx >= len(cfg.Projects) {
					configMu.Unlock()
					continue
				}
				p := cfg.Projects[idx]
				configMu.Unlock()

				// Perform checks
				sources, err := GetCycleSources(p.Path)
				if err != nil {
					eventChan <- BackgroundEvent{Type: "ERROR", Message: fmt.Sprintf("Failed to list sources: %v", err)}
					window.Invalidate()
					continue
				}

				active, err := DetectActiveSource(p.Path, p.SymlinkName)
				if err != nil && err != ErrRealDirectory {
					eventChan <- BackgroundEvent{Type: "ERROR", Message: fmt.Sprintf("Failed checking active link: %v", err)}
				}

				msg := ""
				if err == ErrRealDirectory {
					msg = "WARNING: Target path is a real directory, not a symlink! Overwriting is blocked."
				}

				eventChan <- BackgroundEvent{
					Type:          "SCAN_RESULT",
					Profiles:      sources,
					ActiveProfile: active,
					Message:       msg,
				}
				window.Invalidate()

			case <-time.After(2 * time.Second):
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
					case "ERROR":
						uiState.ActiveError = bEv.Message
						uiState.ActiveSuccess = ""
					case "SUCCESS":
						uiState.ActiveSuccess = bEv.Message
						uiState.ActiveError = ""
					}
				default:
					break
				}
				if len(eventChan) == 0 {
					break
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

func handleInputs(gtx layout.Context, ui *UIState, cfg *Config, triggerScan chan int, eventChan chan BackgroundEvent, window *app.Window) {
	// 1. Add Project Button Action
	if ui.AddProjectBtn.Clicked(gtx) {
		name := strings.TrimSpace(ui.NameInput.Text())
		path := strings.TrimSpace(ui.PathInput.Text())
		symlink := strings.TrimSpace(ui.SymlinkInput.Text())
		denylistRaw := strings.TrimSpace(ui.DenylistInput.Text())

		if name == "" || path == "" || symlink == "" {
			eventChan <- BackgroundEvent{Type: "ERROR", Message: "Name, path, and target link name are required."}
		} else {
			// Check path existence
			if _, err := os.Stat(path); err != nil {
				eventChan <- BackgroundEvent{Type: "ERROR", Message: fmt.Sprintf("Project path does not exist: %v", err)}
			} else {
				// Parse comma-separated processes
				var denylist []string
				for _, p := range strings.Split(denylistRaw, ",") {
					p = strings.TrimSpace(p)
					if p != "" {
						denylist = append(denylist, p)
					}
				}

				p := Project{
					Name:            name,
					Path:            path,
					SymlinkName:     symlink,
					ProcessDenylist: denylist,
				}

				cfg.Projects = append(cfg.Projects, p)
				if err := SaveConfigAtomic(cfg); err != nil {
					eventChan <- BackgroundEvent{Type: "ERROR", Message: fmt.Sprintf("Failed to save config: %v", err)}
				} else {
					ui.NameInput.SetText("")
					ui.PathInput.SetText("")
					ui.DenylistInput.SetText("")
					newIdx := len(cfg.Projects) - 1
					ui.ActiveProjectIndex.Store(int32(newIdx))
					eventChan <- BackgroundEvent{Type: "SUCCESS", Message: "Project added successfully."}
					triggerScan <- newIdx
				}
			}
		}
	}

	// 2. Select Project List Item
	if len(ui.ProjectButtons) != len(cfg.Projects) {
		ui.ProjectButtons = make([]widget.Clickable, len(cfg.Projects))
	}
	for i := range ui.ProjectButtons {
		if ui.ProjectButtons[i].Clicked(gtx) {
			ui.ActiveProjectIndex.Store(int32(i))
			ui.ActiveError = ""
			ui.ActiveSuccess = ""
			ui.AvailableProfiles = nil
			ui.DetectedActive = ""
			triggerScan <- i
		}
	}

	// 3. Delete Project
	activeIdx := int(ui.ActiveProjectIndex.Load())
	if ui.DeleteBtn.Clicked(gtx) && activeIdx >= 0 {
		cfg.Projects = append(cfg.Projects[:activeIdx], cfg.Projects[activeIdx+1:]...)
		ui.ActiveProjectIndex.Store(-1)
		ui.AvailableProfiles = nil
		ui.DetectedActive = ""
		if err := SaveConfigAtomic(cfg); err != nil {
			eventChan <- BackgroundEvent{Type: "ERROR", Message: fmt.Sprintf("Failed to save config: %v", err)}
		} else {
			eventChan <- BackgroundEvent{Type: "SUCCESS", Message: "Project deleted."}
		}
	}

	// 4. Trigger Manual Refresh
	if ui.RefreshBtn.Clicked(gtx) && activeIdx >= 0 {
		ui.ActiveSuccess = ""
		triggerScan <- activeIdx
	}

	// 5. Account Switch Action Buttons
	for i := range ui.SwitchButtons {
		if ui.SwitchButtons[i].Clicked(gtx) && activeIdx >= 0 {
			target := ui.AvailableProfiles[i]
			proj := cfg.Projects[activeIdx]
			
			// Execute symlink swap in background to prevent UI jank
			go func(p Project, t string, idx int) {
				err := SwitchActiveSource(p, t)
				if err != nil {
					eventChan <- BackgroundEvent{Type: "ERROR", Message: err.Error()}
				} else {
					// Update local config state
					configMu.Lock()
					cfg.Projects[idx].CurrentActive = t
					_ = SaveConfigAtomic(cfg)
					configMu.Unlock()

					eventChan <- BackgroundEvent{Type: "SUCCESS", Message: fmt.Sprintf("Switched to target %q", t)}
					triggerScan <- idx
				}
				window.Invalidate()
			}(proj, target, activeIdx)
		}
	}
}

// GUI LAYOUTS
func layoutMain(gtx layout.Context, th *material.Theme, ui *UIState, cfg *Config) {
	// Sidebar occupies 20% of screen width (max 200dp, min 150dp)
	sidebarWidth := gtx.Dp(180)

	// Wrap entire UI in horizontal layout
	layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = sidebarWidth
			gtx.Constraints.Min.X = sidebarWidth
			return drawSidebar(gtx, th, ui, cfg)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return drawContent(gtx, th, ui, cfg)
		}),
	)
}

func drawSidebar(gtx layout.Context, th *material.Theme, ui *UIState, cfg *Config) layout.Dimensions {
	// Fill background color
	paint.FillShape(gtx.Ops, colorSidebar, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Inset{Top: unit.Dp(16), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			activeIdx := int(ui.ActiveProjectIndex.Load())
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.H6(th, "ZEN-CYCLE")
					lbl.Color = colorAccent
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(th, "ACCOUNT MANAGEMENT")
					lbl.Color = colorSubtext
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
				
				// Scrollable project list
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					list := layout.List{Axis: layout.Vertical}
					return list.Layout(gtx, len(cfg.Projects), func(gtx layout.Context, i int) layout.Dimensions {
						proj := cfg.Projects[i]
						
						// Highlight selected project
						btnColor := color.NRGBA{R: 0, G: 0, B: 0, A: 0}
						txtColor := colorText
						if i == activeIdx {
							btnColor = color.NRGBA{R: 0xff, G: 0x6b, B: 0x00, A: 0x33} // light transparent amber
							txtColor = colorAccent
						}

						return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return material.Clickable(gtx, &ui.ProjectButtons[i], func(gtx layout.Context) layout.Dimensions {
								// Background block
								defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
								paint.FillShape(gtx.Ops, btnColor, clip.Rect{Max: gtx.Constraints.Min}.Op())
								
								return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Body2(th, proj.Name)
									lbl.Color = txtColor
									lbl.Font.Weight = font.Medium
									return lbl.Layout(gtx)
								})
							})
						})
					})
				}),
			)
		},
	)
}

func drawContent(gtx layout.Context, th *material.Theme, ui *UIState, cfg *Config) layout.Dimensions {
	// Fill content background
	paint.FillShape(gtx.Ops, colorBg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Inset{Top: unit.Dp(20), Bottom: unit.Dp(20), Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			activeIdx := int(ui.ActiveProjectIndex.Load())
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Header Status Bar
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return drawStatusBar(gtx, th, ui)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

				// Main Content Panels: either detailed project view, or a fallback add form
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if activeIdx >= 0 && activeIdx < len(cfg.Projects) {
						return drawProjectDetail(gtx, th, ui, cfg.Projects[activeIdx])
					}
					return drawAddProjectForm(gtx, th, ui)
				}),
			)
		},
	)
}

func drawStatusBar(gtx layout.Context, th *material.Theme, ui *UIState) layout.Dimensions {
	if ui.ActiveError != "" {
		return drawAlert(gtx, th, ui.ActiveError, colorRed)
	}
	if ui.ActiveSuccess != "" {
		return drawAlert(gtx, th, ui.ActiveSuccess, colorGreen)
	}
	return layout.Dimensions{}
}

func drawAlert(gtx layout.Context, th *material.Theme, textVal string, bg color.NRGBA) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, color.NRGBA{R: bg.R, G: bg.G, B: bg.B, A: 0x22}, clip.Rect{Max: gtx.Constraints.Min}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(th, textVal)
					lbl.Color = bg
					return lbl.Layout(gtx)
				})
			}),
		)
	})
}

func drawProjectDetail(gtx layout.Context, th *material.Theme, ui *UIState, p Project) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					lbl := material.H5(th, p.Name)
					lbl.Color = colorText
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(th, &ui.RefreshBtn, "SCAN")
					btn.Background = colorSidebar
					btn.Color = colorText
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(th, &ui.DeleteBtn, "REMOVE")
					btn.Background = colorRed
					btn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
					return btn.Layout(gtx)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		// Stats Metadata Info Block
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, colorSidebar, clip.Rect{Max: gtx.Constraints.Min}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Body2(th, "Location:  ")
										lbl.Color = colorSubtext
										return lbl.Layout(gtx)
									}),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										lbl := material.Body2(th, p.Path)
										lbl.Color = colorText
										return lbl.Layout(gtx)
									}),
								)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Body2(th, "Symlink Point:  ")
										lbl.Color = colorSubtext
										return lbl.Layout(gtx)
									}),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										lbl := material.Body2(th, filepath.Join(p.Path, p.SymlinkName))
										lbl.Color = colorText
										return lbl.Layout(gtx)
									}),
								)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Body2(th, "Process Locks:  ")
										lbl.Color = colorSubtext
										return lbl.Layout(gtx)
									}),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										val := "None configured"
										if len(p.ProcessDenylist) > 0 {
											val = strings.Join(p.ProcessDenylist, ", ")
										}
										lbl := material.Body2(th, val)
										lbl.Color = colorText
										return lbl.Layout(gtx)
									}),
								)
							}),
						)
					})
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

		// Profile Selection List Header
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body1(th, "AVAILABLE CYCLE SOURCES (.zen-cycle)")
			lbl.Color = colorAccent
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

		// Profile List
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(ui.AvailableProfiles) == 0 {
				lbl := material.Caption(th, "No accounts/profiles detected in .zen-cycle. Manually add subfolders first.")
				lbl.Color = colorSubtext
				return lbl.Layout(gtx)
			}

			list := layout.List{Axis: layout.Vertical}
			return list.Layout(gtx, len(ui.AvailableProfiles), func(gtx layout.Context, i int) layout.Dimensions {
				profile := ui.AvailableProfiles[i]
				isActive := (profile == ui.DetectedActive)

				bgVal := colorSidebar
				if isActive {
					bgVal = color.NRGBA{R: 0x2d, G: 0x2d, B: 0x2d, A: 0xff}
				}

				return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Stack{}.Layout(gtx,
						layout.Expanded(func(gtx layout.Context) layout.Dimensions {
							paint.FillShape(gtx.Ops, bgVal, clip.Rect{Max: gtx.Constraints.Min}.Op())
							return layout.Dimensions{Size: gtx.Constraints.Min}
						}),
						layout.Stacked(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												lbl := material.Body1(th, profile)
												lbl.Color = colorText
												lbl.Font.Weight = font.Medium
												return lbl.Layout(gtx)
											}),
											layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												if isActive {
													lbl := material.Caption(th, "[ACTIVE]")
													lbl.Color = colorAccent
													lbl.Font.Weight = font.Bold
													return lbl.Layout(gtx)
												}
												return layout.Dimensions{}
											}),
										)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if isActive {
											btn := material.Button(th, &ui.SwitchButtons[i], "ACTIVE")
											btn.Background = color.NRGBA{R: 0x44, G: 0x44, B: 0x44, A: 0xff}
											btn.Color = colorSubtext
											return btn.Layout(gtx)
										}
										btn := material.Button(th, &ui.SwitchButtons[i], "SWITCH TO")
										btn.Background = colorAccent
										btn.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
										return btn.Layout(gtx)
									}),
								)
							})
						}),
					)
				})
			})
		}),
	)
}

func drawAddProjectForm(gtx layout.Context, th *material.Theme, ui *UIState) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.H5(th, "Add Project")
			lbl.Color = colorText
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(th, "Configure a folder to enable profile/account cycling via symlink points.")
			lbl.Color = colorSubtext
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),

		// Name Input Field
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body2(th, "Project Name")
			lbl.Color = colorSubtext
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			edt := material.Editor(th, &ui.NameInput, "e.g., Discord Accounts")
			return drawInputField(gtx, edt)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		// Path Input Field
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body2(th, "Project Folder Path (Absolute Directory Path)")
			lbl.Color = colorSubtext
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			edt := material.Editor(th, &ui.PathInput, "e.g., /home/user/my_app")
			return drawInputField(gtx, edt)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		// Symlink Target Name Input
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body2(th, "Target Link Folder Name (will be created as Symlink)")
			lbl.Color = colorSubtext
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			edt := material.Editor(th, &ui.SymlinkInput, "default: profile")
			return drawInputField(gtx, edt)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		// Process lock/denylist Input
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body2(th, "Process Denylist (comma separated, guards against split-brain writes)")
			lbl.Color = colorSubtext
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			edt := material.Editor(th, &ui.DenylistInput, "e.g. discord.exe, discord")
			return drawInputField(gtx, edt)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

		// Submit Button
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			btn := material.Button(th, &ui.AddProjectBtn, "ADD PROJECT")
			btn.Background = colorAccent
			btn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			return btn.Layout(gtx)
		}),
	)
}

func drawInputField(gtx layout.Context, edt material.EditorStyle) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, colorSidebar, clip.Rect{Max: gtx.Constraints.Min}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return edt.Layout(gtx)
			})
		}),
	)
}
