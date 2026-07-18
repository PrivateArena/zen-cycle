package ui

import (
	"image/color"
	"path/filepath"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"zen-cycle/pkg/config"
)

// GUI LAYOUTS
func layoutMain(gtx layout.Context, th *material.Theme, ui *UIState, cfg *config.Config) {
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

func drawSidebar(gtx layout.Context, th *material.Theme, ui *UIState, cfg *config.Config) layout.Dimensions {
	// Fill background color
	paint.FillShape(gtx.Ops, colorSidebar, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Inset{Top: unit.Dp(16), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			activeIdx := int(ui.ActiveProjectIndex.Load())
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(th, "ZEN-CYCLE")
					lbl.Color = colorAccent
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(th, &ui.HomeBtn, "HOME")
					btn.Background = color.NRGBA{R: 0x2a, G: 0x2a, B: 0x2a, A: 0xff}
					btn.Color = colorAccent
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),

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
								return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return layout.Stack{}.Layout(gtx,
											layout.Expanded(func(gtx layout.Context) layout.Dimensions {
												paint.FillShape(gtx.Ops, btnColor, clip.Rect{Max: gtx.Constraints.Min}.Op())
												return layout.Dimensions{Size: gtx.Constraints.Min}
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
													layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
														return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															lbl := material.Body2(th, proj.Name)
															lbl.Color = txtColor
															lbl.Font.Weight = font.Medium
															return lbl.Layout(gtx)
														})
													}),
												)
											}),
										)
									}),
								)
							})
						})
					})
				}),
			)
		},
	)
}

func drawContent(gtx layout.Context, th *material.Theme, ui *UIState, cfg *config.Config) layout.Dimensions {
	paint.FillShape(gtx.Ops, colorBg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(20), Bottom: unit.Dp(20), Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					activeIdx := int(ui.ActiveProjectIndex.Load())
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if activeIdx >= 0 && activeIdx < len(cfg.Projects) {
								return drawProjectDetail(gtx, th, ui, cfg.Projects[activeIdx], cfg.ScrollSpeed)
							}
							return drawAddProjectForm(gtx, th, ui)
						}),
					)
				},
			)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if ui.ActiveError != "" || ui.ActiveSuccess != "" {
				return layout.Inset{Top: unit.Dp(8), Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return drawStatusBar(gtx, th, ui)
				})
			}
			return layout.Dimensions{}
		}),
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

func drawProjectDetail(gtx layout.Context, th *material.Theme, ui *UIState, p config.Project, scrollSpeed float64) layout.Dimensions {
	if ui.EditingProject {
		return drawEditProjectForm(gtx, th, ui, p)
	}
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
					btn := material.Button(th, &ui.EditBtn, "EDIT")
					btn.Background = colorSidebar
					btn.Color = colorAccent
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
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

		// Stats Metadata Info Block
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, colorSidebar, clip.Rect{Max: gtx.Constraints.Min}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
							layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
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
							layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
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
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

		// Profile Selection List Header
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body1(th, "AVAILABLE CYCLE SOURCES (.zen-cycle)")
			lbl.Color = colorAccent
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

		// Profile List
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(ui.AvailableProfiles) == 0 {
				lbl := material.Caption(th, "No accounts/profiles detected in .zen-cycle.")
				lbl.Color = colorSubtext
				return lbl.Layout(gtx)
			}

			// On settling frames (after a ScrollBy), just Layout — no detection.
			listLayout := func(gtx layout.Context) layout.Dimensions {
				return ui.ProfileList.Layout(gtx, len(ui.AvailableProfiles), func(gtx layout.Context, i int) layout.Dimensions {
					profile := ui.AvailableProfiles[i]
					isActive := (profile == ui.DetectedActive)

					bgVal := colorSidebar
					if isActive {
						bgVal = color.NRGBA{R: 0x2d, G: 0x2d, B: 0x2d, A: 0xff}
					}

					return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Stack{}.Layout(gtx,
							layout.Expanded(func(gtx layout.Context) layout.Dimensions {
								paint.FillShape(gtx.Ops, bgVal, clip.Rect{Max: gtx.Constraints.Min}.Op())
								return layout.Dimensions{Size: gtx.Constraints.Min}
							}),
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													lbl := material.Body2(th, profile)
													lbl.Color = colorText
													lbl.Font.Weight = font.Medium
													return lbl.Layout(gtx)
												}),
												layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													if isActive {
														lbl := material.Caption(th, "ACTIVE")
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
											btn := material.Button(th, &ui.SwitchButtons[i], "SWITCH")
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
			}

			// On settling frames (after a ScrollBy), just Layout — no detection.
			if ui.ScrollSettling {
				ui.ScrollSettling = false
				return listLayout(gtx)
			}

			prevFirst := ui.ProfileList.Position.First
			prevOffset := ui.ProfileList.Position.Offset

			dims := listLayout(gtx)

			m := scrollSpeed
			if m > 0 && m != 1.0 {
				n := len(ui.AvailableProfiles)
				pos := ui.ProfileList.Position
				if n > 0 && pos.Length > 0 {
					avgItemPx := float64(pos.Length) / float64(n)
					pixelDelta := float64(pos.First-prevFirst)*avgItemPx + float64(pos.Offset-prevOffset)
					if pixelDelta > 0.5 || pixelDelta < -0.5 {
						extraPx := pixelDelta * (m - 1.0)
						// Cap at remaining scrollable distance to avoid clamp bounce
						if pixelDelta > 0 {
							remaining := float64(n-pos.First)*avgItemPx - float64(pos.Offset)
							if extraPx > remaining {
								extraPx = remaining
							}
						} else {
							available := float64(pos.First)*avgItemPx + float64(pos.Offset)
							if -extraPx > available {
								extraPx = -available
							}
						}
						extraItems := float32(extraPx / avgItemPx)
						if extraItems != 0 {
							ui.ProfileList.ScrollBy(extraItems)
							ui.ScrollSettling = true
							gtx.Execute(op.InvalidateCmd{})
						}
					}
				}
			}
			return dims
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

func drawEditProjectForm(gtx layout.Context, th *material.Theme, ui *UIState, p config.Project) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.H5(th, "Edit Project")
			lbl.Color = colorText
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(th, "Modify the project configuration.")
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
			edt := material.Editor(th, &ui.EditNameInput, "e.g., Discord Accounts")
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
			edt := material.Editor(th, &ui.EditPathInput, "e.g., /home/user/my_app")
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
			edt := material.Editor(th, &ui.EditSymlinkInput, "default: profile")
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
			edt := material.Editor(th, &ui.EditDenylistInput, "e.g. discord.exe, discord")
			return drawInputField(gtx, edt)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

		// Submit / Cancel Buttons
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(th, &ui.SaveEditBtn, "SAVE")
					btn.Background = colorAccent
					btn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(th, &ui.CancelEditBtn, "CANCEL")
					btn.Background = colorSidebar
					btn.Color = colorText
					return btn.Layout(gtx)
				}),
			)
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
