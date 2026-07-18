# zen-cycle Go

## Purpose
zen-cycle is a Go terminal/desktop UI application built with Gio for managing development project cycles via symlinks and Windows junctions. It allows developers to switch active project sources, detect cycle directories, and persist configuration atomically for cross-platform development workflows.

```
main.go → RunUI(pkg/ui/run.go)
pkg/ui/run.go → handleInputs(pkg/ui/inputs.go)
pkg/ui/run.go → layoutMain(pkg/ui/layout.go)
pkg/ui/run.go → LoadConfig(pkg/config/config.go)
pkg/ui/inputs.go → SwitchActiveSource(pkg/link/link.go)
pkg/link/link.go → createLink, removeReparsePoint, IsNTFS
pkg/config/config.go → SaveConfigAtomic, backupCorruptConfig
```

## File Tree
```
main.go
pkg/config/config.go
pkg/config/config_test.go
pkg/link/link.go
pkg/link/link_test.go
pkg/link/process_darwin.go
pkg/link/process_linux.go
pkg/link/process_windows.go
pkg/ui/run.go
pkg/ui/inputs.go
pkg/ui/layout.go
pkg/ui/types.go
pkg/ui/toast.go
```

## Component Roles
| File / Module | Role | Key Exports |
|---|---|---|
| main.go | Entry point; bootstraps Gio app and loads config | main |
| pkg/ui/run.go | Top-level UI loop and window lifecycle | RunUI |
| pkg/ui/layout.go | UI rendering: sidebar, content, forms, status bar, alerts | layoutMain, drawSidebar, drawContent, drawStatusBar, drawAlert, drawProjectDetail, drawAddProjectForm, drawEditProjectForm, drawInputField |
| pkg/ui/inputs.go | Input event dispatch and UI state transitions | handleInputs |
| pkg/ui/types.go | UI state machine and background event types | UIState, BackgroundEvent |
| pkg/ui/toast.go | Toast notification queue, expiry, and overlay rendering | Toast, ToastQueue, ToastType, Push, DrainExpired, DrawToast, DrawToastOverlay |
| pkg/config/config.go | Configuration loading, atomic save, path resolution, corruption recovery | Config, Project, LoadConfig, SaveConfigAtomic, getBinaryDir, getConfigPaths, backupCorruptConfig |
| pkg/link/link.go | Symlink and junction lifecycle management for source switching | GetCycleSources, DetectActiveSource, SwitchActiveSource, CreateLink, CheckDenylistProcesses, IsProcessRunning |
| pkg/link/process_darwin.go | macOS process denylist detection | IsProcessRunning |
| pkg/link/process_linux.go | Linux process denylist detection | IsProcessRunning |
| pkg/link/process_windows.go | Windows process denylist detection | IsProcessRunning |
| pkg/config/config_test.go | Tests for atomic config writes and path resolution | TestAtomicConfig |
| pkg/link/link_test.go | Tests for symlink, junction, and process denylist behavior | TestSymlinkJunction |

## Key Architectural Patterns
1. Gio UI as orchestrator: pkg/ui/run.go owns the app lifecycle, delegating rendering to layout.go, input handling to inputs.go, and persistence to pkg/config.
2. Platform-aware link abstraction: pkg/link unifies POSIX symlinks and Windows junctions via build-tagged files, with IsNTFS guarding junction operations.
3. Atomic config safety: SaveConfigAtomic writes to a temp file then renames, with backupCorruptConfig providing rollback on parse failure.
4. Toast overlay system: pkg/ui/toast.go implements a queue-based notification layer with expiry draining and floating overlay rendering in the content area.

## Dependencies
| Package / Module | Role |
|---|---|
| gioui.org v0.10.1 | Cross-platform GPU-accelerated UI toolkit providing windows, input, and drawing primitives |
| gioui.org/shader v1.0.8 | Shader utilities for Gio rendering pipeline |
| github.com/go-text/typesetting v0.3.4 | Advanced text shaping and font fallback for Gio |
| golang.org/x/image v0.26.0 | Image format support and processing |
| golang.org/x/net v0.48.0 | Network utilities (indirect) |
| golang.org/x/sys v0.39.0 | OS-level system calls for symlink and process detection |
| golang.org/x/text v0.32.0 | Unicode and text processing (indirect) |
| golang.org/x/exp/shiny | Experimental OpenGL driver for Gio on supported platforms |
