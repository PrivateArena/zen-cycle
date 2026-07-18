package ui

import (
	"image/color"
	"sync/atomic"

	"gioui.org/layout"
	"gioui.org/widget"
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

	// Editors for Editing Projects
	EditingProject   bool
	EditNameInput    widget.Editor
	EditPathInput    widget.Editor
	EditSymlinkInput widget.Editor
	EditDenylistInput widget.Editor

	// Buttons
	HomeBtn        widget.Clickable
	AddProjectBtn  widget.Clickable
	RefreshBtn     widget.Clickable
	EditBtn        widget.Clickable
	SaveEditBtn    widget.Clickable
	CancelEditBtn  widget.Clickable
	DeleteBtn      widget.Clickable
	SwitchButtons  []widget.Clickable
	ProjectButtons []widget.Clickable

	// Persistent layout state
	ProfileList layout.List

	ScrollSettling bool // debounce: true when ScrollBy was applied last frame
}

// BackgroundEvent represents async updates from worker to UI thread.
type BackgroundEvent struct {
	Type          string   // "SCAN_RESULT", "ERROR", "SUCCESS", "SWITCH_DONE"
	Index         int      // project index for targeted updates
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
