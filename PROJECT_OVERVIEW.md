# zen-cycle Go

## Purpose
zen-cycle is a Go terminal UI (TUI) application for managing development project cycles via symlinks and Windows junctions. It allows developers to switch active project sources, detect cycle directories, and persist configuration atomically for cross-platform development workflows.

```
main.go → runUI(ui.go)
main.go → LoadConfig(config.go)
ui.go → SwitchActiveSource(link.go)
ui.go → SaveConfigAtomic(config.go)
link.go → createLink, removeReparsePoint, IsNTFS
config.go → getBinaryDir, getConfigPaths, backupCorruptConfig
```

## File Tree
```
config.go
cycle_test.go
link.go
main.go
ui.go
```

## Component Roles
| File / Module | Role | Key Exports |
|---|---|---|
| main.go | Entry point; bootstraps TUI and loads config | main |
| ui.go | Terminal UI engine with sidebar, forms, status bar, and alert rendering | runUI, handleInputs, UIState, BackgroundEvent, drawSidebar, drawContent, drawProjectDetail, drawAddProjectForm, drawInputField, layoutMain, drawStatusBar, drawAlert |
| config.go | Configuration loading, atomic save, binary/path resolution, and corruption recovery | Config, Project, LoadConfig, SaveConfigAtomic, getBinaryDir, getConfigPaths, backupCorruptConfig |
| link.go | Symlink and junction lifecycle management for source switching | GetCycleSources, DetectActiveSource, SwitchActiveSource, createLink, removeReparsePoint, IsNTFS, CheckDenylistProcesses, isProcessRunning |
| cycle_test.go | Tests for atomic config writes, junction behavior, process denylist, and Windows reparse point removal | TestAtomicConfig, TestSymlinkJunction, TestProcessRunningCheck, TestWindowsReparseRemoval |

## Key Architectural Patterns
1. TUI as orchestrator: ui.go owns the event loop and input handling, delegating link mutations to link.go and state persistence to config.go.
2. Platform-aware link abstraction: link.go unifies POSIX symlinks and Windows junctions, with IsNTFS guarding junction operations.
3. Atomic config safety: SaveConfigAtomic writes to a temp file then renames, with backupCorruptConfig providing rollback on parse failure.

## Dependencies
No external dependency manifest (go.mod) was indexed; project uses Go standard library only.
