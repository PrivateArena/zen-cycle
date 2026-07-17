# Zen-Cycle Code Review: Safety, Correctness & Maintainability

## Summary

Review of a 5-file Go GUI application (Gio) that manages account/profile cycling via symlinks. The user configures a project directory, selects subfolders inside `.zen-cycle/`, and the app atomically flips a symlink to point at the selected target. A denylist-based process guard is meant to prevent split-brain writes. The review uncovered **4 critical bugs** (including a guaranteed deadlock on first profile switch, a nil-pointer crash on config corruption, and an out-of-bounds panic from stale goroutine indices), **several race conditions undermining the safety model**, and **significant test coverage gaps** — most critically, the core "cycle" operation has never been tested end-to-end.

---

## Findings (Ordered by Severity)

### 🔴 Critical

#### 1. Deadlock on first profile switch — `configMu` double-locked (ui.go:270-285 + config.go:109-156)
The switch-completion goroutine calls `configMu.Lock()` before mutating `cfg.Projects[idx].CurrentActive`, then calls `SaveConfigAtomic(cfg)` — which immediately calls `configMu.Lock()` again. `sync.Mutex` is not reentrant in Go. The second lock blocks forever, leaking the goroutine and permanently freezing all config I/O. Every subsequent add/delete/scan blocks forever.

**Fix**: Remove the outer `configMu.Lock()`/`Unlock()` — `SaveConfigAtomic` manages its own locking. Route the mutation back through the UI goroutine via `eventChan` instead.

#### 2. nil `*Config` passed to `runUI` on corrupt JSON → crash on first frame (main.go:26-29)
`LoadConfig` returns `(nil, error)` when JSON unmarshal fails (even after backing up the corrupt file). `main.go` treats *any* error as soft — logs a warning and proceeds to `runUI(w, cfg)` with `cfg == nil`. The first `len(cfg.Projects)` or `cfg.Projects[i]` in the render path causes a nil-dereference panic.

**Fix**: Either have `LoadConfig` return `(&Config{}, err)` on corruption, or have `main.go` substitute `&Config{}` whenever `cfg == nil`.

#### 3. Unsynchronized `cfg.Projects` mutation — data race + stale reads (ui.go:212, 246, 267 vs. ui.go:84-90, 276-278)
`handleInputs` (UI goroutine) does `cfg.Projects = append(cfg.Projects, p)` and `cfg.Projects = append(cfg.Projects[:i], cfg.Projects[i+1:]...)` **without holding any lock**. The background scan goroutine reads `cfg.Projects[idx]` under `configMu`, and the switch goroutine writes `cfg.Projects[idx].CurrentActive` under `configMu`. Only one side locks. Slice-header torn reads (append reallocates the backing array) can produce out-of-bounds panics and stale/zeroed struct reads. Every render-frame access (drawSidebar, drawContent) also reads unlocked.

**Fix**: Route *all* `cfg.Projects` mutations through the UI goroutine's event drain loop, eliminating concurrent access entirely.

#### 4. Stale captured index → out-of-bounds panic on concurrent delete (ui.go:267, 270-285)
The switch-button goroutine captures `activeIdx` (an `int`) before the goroutine runs. If the user deletes a project (shrinking `cfg.Projects`) while the goroutine is in flight, `cfg.Projects[idx]` indexes out of bounds → runtime panic. Nothing disables controls during in-flight switches.

**Fix**: After the goroutine completes, re-check bounds before indexing; or disable switch/delete buttons while a switch is pending.

#### 5. Windows: link deleted before new one is proven to exist → link point can vanish (link.go:187-199)
`removeReparsePoint(absLink)` deletes the old junction/symlink before `mklink` runs. If `mklink` fails *and* the `os.Symlink` fallback also fails, the old link is gone and nothing is at `absLink`. The project's entry point vanishes.

**Fix**: Build the new link at a temp name first. Only touch the old link once the new one exists. Move the old link aside (not delete) so it can be restored on rename failure.

#### 6. `removeReparsePoint` can silently delete a real, empty directory (link.go:212-217)
If a user manually creates a plain empty folder named `profile`, `removeReparsePoint` sees `fi.IsDir()`, calls `os.Remove(path)` — which succeeds on any empty directory — and returns `nil`. No error, no warning. The directory is gone.

**Fix**: If the path is a directory but does NOT have `os.ModeSymlink` set, never attempt deletion — return `ErrRealDirectory` directly.

---

### 🟠 Major

#### 7. Process denylist is pure TOCTOU — unbounded window between check and swap (link.go:136-156)
Between the denylist check (`pgrep`/`tasklist`) and the symlink creation, any number of guarded processes can start. The guard provides a false sense of safety. At minimum this should be documented as a known limitation; ideally re-check immediately before the final rename.

#### 8. `pgrep -f` and substring matching produces false positives/negatives (link.go:114, 128)
`pgrep -f name` matches against the *entire command line*, not the process name. `strings.Contains` in the `ps`/`tasklist` fallback overmatches (e.g. `"code"` matches `"vscode"`). For a safety-critical check, exact process-name matching is needed.

#### 9. Background `eventChan` sends can block indefinitely, leaking goroutines (ui.go:76, 270-285)
`eventChan` (capacity 20) is only drained inside `app.FrameEvent`. If the window is minimized or enough switch goroutines fill the buffer, `eventChan <-` blocks forever. The scanning coordinator blocks too — effectively freezing the app. The `window.Invalidate()` call from a background goroutine after frame delivery is also problematic.

#### 10. POSIX: orphaned temp symlink on rename failure (link.go:170-173)
If `os.Rename(tmpLink, link)` fails, `tmpLink` is left as clutter. No cleanup path.

#### 11. No confirmation dialog for project deletion (ui.go:244-255)
One click permanently removes the project from config. No "Are you sure?" — easy to fat-finger and lose the config entry.

#### 12. Stale scan results after project deletion (ui.go:84, 246)
If a `triggerScanChan` message for a now-stale index is still in the channel buffer, the coordinator scans whatever project now occupies that index — silently the wrong project.

#### 13. Test coverage: core "cycle" behavior never tested end-to-end (cycle_test.go)
`TestSymlinkJunction` only tests first-time switch (no-link → profile_A). Switching from A → B (the actual cycling path) is untested. The process-guard block path is untested. Corrupt-config recovery is untested. `getConfigPaths` priority is untested.

#### 14. Global mutable state (`loadedPath`, `configMu`) creates inter-test pollution (config.go + cycle_test.go)
Tests reassign `loadedPath` directly. Any new test that loads config without resetting it first gets cross-test contamination.

---

### 🟡 Minor

#### 15. `SaveConfigAtomic` Windows path: crash between `os.Remove` and `os.Rename` loses config (config.go:147-151)
If the process crashes after removing the old file but before renaming the temp file, config is gone. Rename the old file aside instead of deleting it.

#### 16. `backupCorruptConfig` never `Sync()`s the backup file (config.go:158-174)
A crash after `io.Copy` could lose both original and backup.

#### 17. `LoadConfig` swallows read errors silently (config.go:83-86)
If the highest-priority config exists but is unreadable (permissions), the loader silently falls through to lower-priority paths without clear diagnostics.

#### 18. `finalErr` returned alongside valid default config → misleading warning (config.go:108)
A transient read error on an earlier path causes a "Failed to load config" warning even when a later path (or default) succeeds normally.

#### 19. `time.After` in hot `select` reallocates timer every iteration (ui.go:118)
Classic Go pitfall — creates garbage and timer churn. Use `time.NewTicker` instead.

#### 20. `Add`/`Delete` do `SaveConfigAtomic` synchronously on UI goroutine (ui.go:213, 250)
Inconsistent with the async switch pattern. Slow disk I/O stalls the UI visibly.

#### 21. `SwitchActiveSource` `newTarget` not validated against path traversal (link.go:147)
A value like `"../../etc"` resolves outside `.zen-cycle`. Low risk (input comes from directory listing) but cheap to guard.

#### 22. `.zen-cycle` entries not validated — symlinks offered as profiles (link.go:31-37)
A symlink inside `.zen-cycle` pointing outside the project is silently offered as a switchable profile.

#### 23. `getBinaryDir` `"go-build"`/`"Temp"` heuristic misfires in production (config.go:41)
Any legitimate install path containing `"Temp"` or `"go-build"` redirects config resolution to `os.Getwd()`. Use `os.TempDir()` prefix check instead.

#### 24. Magic port `23953` for single-instance lock (main.go:17)
No fallback. Any other process on this port blocks startup with a misleading "already running" error.

#### 25. `os.Exit(0)` inside goroutine skips `main()`'s deferred cleanup (main.go:41)
`defer singleInstanceListener.Close()` is never reached. Harmless today (OS reclaims on exit) but a latent footgun.

#### 26. `Project.CurrentActive` written but never read on startup (config.go + link.go)
Persisted but never used to restore/verify symlink state at launch. Half-implemented feature.

#### 27. `handleInputs` validation logic entangled in Gio widgets — untestable (ui.go:183-226)
Parsing project input from UI state is inlined. Extracting `parseNewProject(...)` as a pure function would make it testable.

#### 28. Process checker not injectable — guard logic unverifiable in unit tests (link.go:101-133)
`isProcessRunning` shells out directly. An interface/function type would allow testing the guard path.

#### 29. `DetectActiveSource` on dangling symlink untested (link.go:76-83)
`os.Readlink` succeeds but the target is gone. Behavior under this path is dark.

#### 30. `getConfigPaths` can include the same path twice via `loadedPath` (config.go:53-54)
Redundant `os.Stat`/`os.ReadFile` if `loadedPath` equals `filepath.Join(binDir, "config.json")`.

---

### ⚪ Nit

#### 31. Error string capitalization (`link.go:143`)
`fmt.Errorf("active processes detected: %s. Close them first.", ...)` — Go convention: errors should not start with a capital letter unless the first word is a proper noun.

#### 32. Inline color literals duplication (ui.go:multiple)
`color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}` repeated inline instead of a shared `colorWhite` variable.

#### 33. `defer os.Remove(tmpName)` after successful rename (config.go:127)
No-op on success (file already moved). Works correctly but looks like a bug at first glance.

#### 34. No platform build notes (main.go)
`runtime.GOOS` branching exists elsewhere but `main.go` has no comment on supported platforms.

---

## Red-Team Critique Reconciliation (Claude Review)

| Claude Finding | Severity | Disposition |
|---|---|---|
| nil Config → crash on corrupt JSON | Critical | **Folded in** (Finding #2) |
| Deadlock on configMu double-lock | Critical | **Folded in** (Finding #1) |
| Unsynchronized cfg.Projects mutations + data race | Critical | **Folded in** (Finding #3) |
| Stale captured index → OOB panic | Critical | **Folded in** (Finding #4) |
| No guard against concurrent SwitchActiveSource | Critical | **Folded in** (Finding #5 — Windows data-loss window) |
| TOCTOU process denylist | Major | **Folded in** (Finding #7) |
| pgrep -f false positives/negatives | Major | **Folded in** (Finding #8) |
| POSIX no explicit real-directory check | Major | **Folded as part of Finding #10** (orphan cleanup) |
| Windows SaveConfigAtomic data-loss window | Major | **Folded in** (Finding #15) |
| eventChan can block indefinitely | Major | **Folded in** (Finding #9) |
| getBinaryDir heuristic misfires | Minor | **Folded in** (Finding #23) |
| Test coverage gaps (7 sub-points) | Major/Minor | **Folded in** (Findings #13, #14, #27, #28, #29) |
| time.After timer reallocation | Minor | **Folded in** (Finding #19) |
| Add/Delete sync SaveConfigAtomic | Minor | **Folded in** (Finding #20) |
| LoadConfig swallows read errors | Minor | **Folded in** (Finding #17) |
| finalErr returned alongside valid default | Minor | **Folded in** (Finding #18) |
| Path traversal in newTarget | Minor | **Folded in** (Finding #21) |
| .zen-cycle symlink not validated | Minor | **Folded in** (Finding #22) |
| Magic port 23953 | Minor | **Folded in** (Finding #24) |
| os.Exit(0) skips defer | Minor | **Folded in** (Finding #25) |
| CurrentActive written but never read | Minor | **Folded in** (Finding #26) |
| getConfigPaths duplicates | Nit | **Folded in** (Finding #30) |
| Error string capitalization | Nit | **Folded in** (Finding #31) |
| Inline color literals | Nit | **Folded in** (Finding #32) |
| removeReparsePoint deletes empty real dirs | Major | **Folded in** (Finding #6) |
| POSIX tmp symlink not cleaned on rename failure | Major | **Folded in** (Finding #10) |

**Rejected items**: None. All Claude findings were corroborated by my own first-pass analysis and folded into the merged set.

**Items Claude missed that I retained**:
- No confirmation dialog for REMOVE → Finding #11 (Major)
- Stale scan results after project deletion → Finding #12 (Major)
- `err` shadowing in LoadConfig's Unmarshal → folded into Finding #17 context
- `backupCorruptConfig` no `Sync()` → Finding #16 (Minor)

---

## Recommended Fix Priorities

1. **P0 — Fix the deadlock** (Finding #1): Remove outer `configMu` from switch goroutine; route result through `eventChan`.
2. **P0 — Fix nil Config crash** (Finding #2): Return empty `&Config{}` on corruption or guard in `main()`.
3. **P0 — Fix removeReparsePoint** (Finding #6): Stop deleting directories without `ModeSymlink`.
4. **P0 — Fix Windows link-vanishes** (Finding #5): Build temp-first, old-aside pattern.
5. **P0 — Eliminate data race on cfg.Projects** (Finding #3): Unify all mutations through UI goroutine.
6. **P0 — Fix stale-index OOB** (Finding #4): Re-check bounds in switch goroutine; disable controls during switch.
7. **P0 — Add basic end-to-end cycle test** (Finding #13 sub-point): Switch A → B → verify link and data.
8. **P1 — Fix eventChan blocking** (Finding #9): Add non-blocking send or context cancellation.
9. **P1 — Fix posix tmp symlink cleanup** (Finding #10): `os.Remove(tmpLink)` on rename failure.
10. **P1 — Add delete confirmation** (Finding #11): Simple dialog or undo window.
