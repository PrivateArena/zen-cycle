package ui

import (
	"fmt"
	"os"
	"strings"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/widget"

	"zen-cycle/pkg/config"
	"zen-cycle/pkg/link"
)

func handleInputs(gtx layout.Context, ui *UIState, cfg *config.Config, triggerScan chan int, eventChan chan BackgroundEvent, window *app.Window) {
	// 0. Home Button — return to Add Project form
	if ui.HomeBtn.Clicked(gtx) {
		ui.ActiveProjectIndex.Store(-1)
		ui.AvailableProfiles = nil
		ui.DetectedActive = ""
		ui.ActiveError = ""
		ui.ActiveSuccess = ""
		ui.EditingProject = false
		ui.Toasts.Clear()
	}

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

				p := config.Project{
					Name:            name,
					Path:            path,
					SymlinkName:     symlink,
					ProcessDenylist: denylist,
				}

				config.ConfigMu.Lock()
				cfg.Projects = append(cfg.Projects, p)
				config.ConfigMu.Unlock()
				if err := config.SaveConfigAtomic(cfg); err != nil {
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
			ui.Toasts.Clear()
			triggerScan <- i
		}
	}

	// 3. Delete Project
	activeIdx := int(ui.ActiveProjectIndex.Load())
	if ui.DeleteBtn.Clicked(gtx) && activeIdx >= 0 {
		config.ConfigMu.Lock()
		cfg.Projects = append(cfg.Projects[:activeIdx], cfg.Projects[activeIdx+1:]...)
		config.ConfigMu.Unlock()
		ui.ActiveProjectIndex.Store(-1)
		ui.AvailableProfiles = nil
		ui.DetectedActive = ""
		if err := config.SaveConfigAtomic(cfg); err != nil {
			eventChan <- BackgroundEvent{Type: "ERROR", Message: fmt.Sprintf("Failed to save config: %v", err)}
		} else {
			eventChan <- BackgroundEvent{Type: "SUCCESS", Message: "Project deleted."}
		}
	}

	// 4. Trigger Manual Refresh
	if ui.RefreshBtn.Clicked(gtx) && activeIdx >= 0 {
		ui.ActiveSuccess = ""
		ui.Toasts.Clear()
		triggerScan <- activeIdx
	}

	// 5. Edit Project
	if ui.EditBtn.Clicked(gtx) && activeIdx >= 0 {
		p := cfg.Projects[activeIdx]
		ui.EditNameInput.SetText(p.Name)
		ui.EditPathInput.SetText(p.Path)
		ui.EditSymlinkInput.SetText(p.SymlinkName)
		ui.EditDenylistInput.SetText(strings.Join(p.ProcessDenylist, ", "))
		ui.EditingProject = true
	}

	// 6. Save Edit
	if ui.SaveEditBtn.Clicked(gtx) && activeIdx >= 0 && ui.EditingProject {
		name := strings.TrimSpace(ui.EditNameInput.Text())
		path := strings.TrimSpace(ui.EditPathInput.Text())
		symlink := strings.TrimSpace(ui.EditSymlinkInput.Text())
		denylistRaw := strings.TrimSpace(ui.EditDenylistInput.Text())

		if name == "" || path == "" || symlink == "" {
			eventChan <- BackgroundEvent{Type: "ERROR", Message: "Name, path, and target link name are required."}
		} else if _, err := os.Stat(path); err != nil {
			eventChan <- BackgroundEvent{Type: "ERROR", Message: fmt.Sprintf("Project path does not exist: %v", err)}
		} else {
			var denylist []string
			for _, p := range strings.Split(denylistRaw, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					denylist = append(denylist, p)
				}
			}
			config.ConfigMu.Lock()
			cfg.Projects[activeIdx].Name = name
			cfg.Projects[activeIdx].Path = path
			cfg.Projects[activeIdx].SymlinkName = symlink
			cfg.Projects[activeIdx].ProcessDenylist = denylist
			config.ConfigMu.Unlock()
			if err := config.SaveConfigAtomic(cfg); err != nil {
				eventChan <- BackgroundEvent{Type: "ERROR", Message: fmt.Sprintf("Failed to save config: %v", err)}
			} else {
				ui.EditingProject = false
				eventChan <- BackgroundEvent{Type: "SUCCESS", Message: "Project updated."}
				triggerScan <- activeIdx
			}
		}
	}

	// 7. Cancel Edit
	if ui.CancelEditBtn.Clicked(gtx) {
		ui.EditingProject = false
	}

	// 8. Account Switch Action Buttons
	for i := range ui.SwitchButtons {
		if ui.SwitchButtons[i].Clicked(gtx) && activeIdx >= 0 {
			target := ui.AvailableProfiles[i]
			proj := cfg.Projects[activeIdx]

			// Execute symlink swap in background to prevent UI jank
			go func(p config.Project, t string, idx int) {
				err := link.SwitchActiveSource(p, t)
				if err != nil {
					eventChan <- BackgroundEvent{Type: "ERROR", Message: err.Error()}
				} else {
					eventChan <- BackgroundEvent{
						Type:          "SWITCH_DONE",
						Index:         idx,
						ActiveProfile: t,
						Message:       fmt.Sprintf("Switched to target %q", t),
					}
					triggerScan <- idx
				}
				window.Invalidate()
			}(proj, target, activeIdx)
		}
	}
}
