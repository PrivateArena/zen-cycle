I have all five files (had to pull `ui.go` directly since it wasn't included in the document content block, but it's now fully reviewed). Here's the full review.

---

## 🔴 Critical

**1. `main.go` — nil `*Config` passed to `runUI` on corrupt config → crash on first frame**
```go
cfg, err := LoadConfig()
if err != nil {
    log.Printf("Warning: Failed to load config, starting with defaults: %v", err)
}
```
`LoadConfig()` returns `nil, fmt.Errorf("config corrupt, backed up: %w", err)` when JSON unmarshal fails (config.go). `main.go` treats *any* error as a soft warning and proceeds to call `runUI(w, cfg)` with `cfg == nil`. The very first render (`len(cfg.Projects)`, `cfg.Projects[i]` in `ui.go`) will nil-deref and crash. This is the most user-visible bug: a single corrupted config file bricks the app on every launch instead of falling back to a fresh default config (which the backup-and-continue design clearly intended).
**Fix:** either have `LoadConfig` return `(&Config{}, err)` on corruption, or have `main.go` explicitly substitute `&Config{}` whenever `cfg == nil`.

**2. `ui.go` — unsynchronized mutation of `cfg.Projects` racing with locked background access**
`handleInputs` (UI goroutine) does:
```go
cfg.Projects = append(cfg.Projects, p)                                   // Add — no lock
cfg.Projects = append(cfg.Projects[:activeIdx], cfg.Projects[activeIdx+1:]...) // Delete — no lock
```
while the scan coordinator goroutine and switch goroutines do:
```go
configMu.Lock()
p := cfg.Projects[idx]     // read
...
cfg.Projects[idx].CurrentActive = t   // write
configMu.Unlock()
```
Only one side takes `configMu`. That's a textbook data race (`go test -race` / `go run -race` will flag it): a slice header replaced by `append` concurrently with an indexed read/write on another goroutine can yield a torn read, a read of freed memory, or worse.

**3. `ui.go` — stale captured index → out-of-bounds panic**
In the switch-button handler:
```go
proj := cfg.Projects[activeIdx]
go func(p Project, t string, idx int) {
    ...
    cfg.Projects[idx].CurrentActive = t
    ...
}(proj, target, activeIdx)
```
`idx` is captured before the goroutine runs. If the user clicks **Delete** on the same (or an earlier-indexed) project while the switch goroutine is still in flight, `cfg.Projects` shrinks and `cfg.Projects[idx]` can now be out of range → runtime panic and app crash. Nothing disables the Delete/Add controls, or the other Switch buttons, while a switch is in-flight.

**4. `link.go` / `ui.go` — no guard against concurrent `SwitchActiveSource` calls on the same project**
Nothing disables `SwitchButtons` after a click, so rapid double-clicks (or clicking two different targets before the first finishes) spawn multiple goroutines racing to create the same `tmpLink` (named only `link + ".tmp-<pid>"` — see next item) and rename it over the same `linkPath`. Combined with the process-denylist check being pure TOCTOU (see Major #1), this is the core safety mechanism the whole tool exists for, and it isn't actually race-safe.

---

## 🟠 Major

**1. `link.go` — process-denylist check is TOCTOU by construction**
```go
activeProcesses, err := CheckDenylistProcesses(p.ProcessDenylist)
...
return createLink(targetPath, linkPath)
```
There's an unbounded window between the check and the actual symlink swap (`pgrep`/`ps`/`tasklist` invocation, JSON I/O, syscalls). If the guarded app starts a split-second after the check passes, the swap proceeds anyway and can corrupt that app's data — which is precisely the scenario `ProcessDenylist` is meant to prevent. Worth documenting as a known limitation at minimum, since it undermines the tool's stated safety guarantee. A tighter (though still imperfect) approach would re-check immediately before the rename, or use file locks that the guarded app also participates in.

**2. `link.go` — `pgrep -f` and substring matching produce false positives/negatives**
`pgrep -f name` matches against the **entire command line**, not just the process name — so denylisting `"notes"` would match any process whose args happen to contain that substring (e.g. `--profile=my-notes-app`), and conversely won't necessarily catch a process invoked in an unexpected way. The `ps`/`tasklist` fallback paths do plain `strings.Contains` substring matching, which has the same overmatch problem (e.g. `"code"` matches `"vscode"`, `"chrome"` matches `"chromium"`). For a safety-critical check, this is worth tightening to exact process-name matching where possible.

**3. `link.go` — POSIX safety against overwriting a real directory relies on undocumented OS behavior, not an explicit check**
Windows explicitly detects `ErrRealDirectory` before removing the reparse point. POSIX has no equivalent check in `SwitchActiveSource`/`createLink` — it relies on `os.Rename(tmpLink, link)` failing because POSIX `rename(2)` refuses to replace a directory with a non-directory. This *happens* to work today, but it's an implicit safety net rather than a designed one, and the error message the user gets (`"failed replacing config file"`-style wrapped rename error) is far less clear than the explicit `ErrRealDirectory` message Windows gives. Recommend adding an explicit `DetectActiveSource`/directory check on POSIX too, for consistent behavior and clearer errors.

**4. `config.go` — Windows save path has a data-loss window**
```go
if runtime.GOOS == "windows" {
    _ = os.Remove(loadedPath) // Remove destination first...
}
if err := os.Rename(tmpName, loadedPath); err != nil { ... }
```
If the process crashes/is killed between the `Remove` and the `Rename` (or the `Rename` itself fails after the `Remove` succeeded), the config file is gone and the temp file is orphaned. Since `SaveConfigAtomic` is called synchronously from UI handlers (add/delete project), a crash or forced-kill during that narrow window loses the entire project list. Consider writing to a distinct backup path first, or renaming the old file aside instead of deleting it, so there's always a recoverable copy.

**5. `ui.go` — background goroutine channel sends can block indefinitely, leaking goroutines**
`eventChan` has a fixed buffer of 20 and is drained only inside `app.FrameEvent` handling. If the window is minimized/occluded on a platform where Gio pauses frame delivery, or if enough switch-goroutines are spawned concurrently (see Critical #4) to fill the buffer, further `eventChan <- ...` sends block forever. Those goroutines (and the single scan coordinator goroutine, which also blocks on `eventChan <-`) never get cleaned up — a goroutine leak, and it also stalls all future scanning since the coordinator's single loop is blocked on the send.

**6. `main.go` — `getBinaryDir`'s `"go-build"`/`"Temp"` heuristic can misfire in production**
```go
if strings.Contains(exe, "go-build") || strings.Contains(dir, "Temp") {
    dir, _ = os.Getwd()
}
```
Any legitimate install path containing the substring `"Temp"` (e.g. a portable app run from `C:\Users\Alice\AppData\Local\Temp-Apps\ZenCycle`, or a folder literally named `Templates`) silently redirects config resolution to the current working directory instead of the binary's directory — which could mean the app "loses" its config depending on what directory it's launched from. This heuristic should be narrowed (e.g. exact `os.TempDir()` prefix check) or dropped in favor of relying on `os.UserConfigDir()` as the stable fallback.

**7. Test coverage gaps**
- No test exercises the corrupt-JSON path in `LoadConfig` (which would have caught Critical #1).
- No test for concurrent `SwitchActiveSource` calls on the same project (would have caught Critical #4).
- `TestProcessRunningCheck` only asserts the *negative* case (nonexistent process not found) — there's no test confirming a genuinely running process *is* detected, so a regression that always returns `false` would pass silently.
- No test for path-traversal-style input to `SwitchActiveSource`'s `newTarget` (e.g. `"../../etc"`), see Minor/security note below.

---

## 🟡 Minor

**1. `ui.go` — `time.After` inside a hot `select` loop reallocates a timer every iteration**
```go
select {
case idx := <-triggerScanChan:
    ...
case <-time.After(2 * time.Second):
    ...
}
```
A new timer is allocated on every loop iteration regardless of which branch fires (classic Go `select`+`time.After` pitfall). Under frequent `triggerScanChan` traffic this creates unnecessary garbage and timer churn. Use a single `time.NewTicker(2*time.Second)` created once outside the loop instead.

**2. `ui.go` — Add/Delete perform `SaveConfigAtomic` synchronously on the UI goroutine**
The switch action is explicitly backgrounded "to prevent UI jank," but Add/Delete call `SaveConfigAtomic` (disk I/O + fsync) directly inside `handleInputs`, which runs on the render path. Inconsistent, and a slow disk (network drive, antivirus scan hook on Windows) will visibly stall the UI on every add/delete.

**3. `config.go` — `LoadConfig` swallows read errors and moves to the next candidate**
```go
data, err := os.ReadFile(absPath)
if err != nil {
    finalErr = err
    continue
}
```
If the highest-priority config file exists but is unreadable (e.g. permissions), the loader silently falls through to lower-priority paths (or defaults) rather than surfacing the problem clearly. The user could be editing/expecting one config while the app is silently using a different one or empty defaults.

**4. `config.go` — `finalErr` can be returned alongside a valid default config, producing a misleading warning**
If an earlier candidate path had a transient `ReadFile` error but a later path (or the default) succeeds, `LoadConfig` still returns the earlier `finalErr`, causing `main.go` to log "Warning: Failed to load config, starting with defaults" even though a real config might exist and just have an unrelated hiccup on another path. Confusing diagnostics.

**5. `link.go` — `SwitchActiveSource`'s `newTarget` isn't validated against path traversal**
```go
targetPath := filepath.Join(p.Path, ".zen-cycle", newTarget)
```
Normally `newTarget` comes from `GetCycleSources` (safe, directory-entry names only), but `SwitchActiveSource` is an exported function with no internal guard — a value like `"../../etc"` would resolve outside `.zen-cycle`. Low severity given this is a local single-user tool with no untrusted input path currently, but worth a defensive check (reject `..`/absolute paths) given the note about symlink/path-traversal hygiene.

**6. `link.go` — no validation that a `.zen-cycle` entry isn't itself a symlink to an arbitrary location**
`GetCycleSources` returns any non-dotfile entry name, including symlinks. A stray symlink placed inside `.zen-cycle` pointing outside the project would be silently offered as a switchable "profile" and then symlinked into as the active target. Minor given local-only trust model, but worth a comment/guard if this tool is ever used with any shared/synced folder (Dropbox, etc.) where such an entry could appear unexpectedly.

**7. `main.go` — hardcoded single-instance port (`23953`) has no fallback/override**
Any other process (or another local app) that happens to be bound to that port at the wrong moment blocks Zen-Cycle from starting, with only a generic "already running" message — no way to tell the two apart. Minor UX/robustness issue.

**8. `main.go` — `os.Exit(0)` after `runUI` returns skips `main()`'s deferred listener close**
```go
if err := runUI(w, cfg); err != nil {
    log.Fatal(err)
}
os.Exit(0)
```
Called from the inner goroutine, `os.Exit(0)` terminates the process immediately, bypassing `defer singleInstanceListener.Close()` in `main()`. Harmless today (OS reclaims the socket on process exit), but it's a latent footgun if any other cleanup-via-defer is added later.

**9. `config.go` — `Project.CurrentActive` is written but never read**
The field is populated on switch (`cfg.Projects[idx].CurrentActive = t`) and persisted, but nothing on startup uses it to restore/verify the symlink state. Looks like a half-implemented feature (e.g. "detect drift between config and actual symlink at launch") — either wire it up or drop it to avoid confusing future maintainers.

**10. `config.go` — `getConfigPaths` can include the same path twice**
If `loadedPath` happens to equal `filepath.Join(binDir, "config.json")`, both get added to the candidate slice, causing one redundant `os.Stat`/`os.ReadFile` pass. Harmless but pointless.

---

## ⚪ Nit

- `link.go`: `fmt.Errorf("active processes detected: %s. Close them first.", ...)` — error strings conventionally shouldn't be capitalized/punctuated per Go style (`go vet`/`golint` will flag this).
- `config.go`: `SaveConfigAtomic`'s `defer os.Remove(tmpName)` after a successful rename will simply no-op (file already moved) — fine, but worth a comment so it doesn't look like a bug on first read.
- `ui.go`: magic color literals (`0x12, 0x12, 0x12`, `0xff6b00`, etc.) are named well at the top, but a few are re-declared inline (e.g. `color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}` for button text) instead of reusing a shared `colorWhite` — trivial duplication.
- `cycle_test.go`: `TestSymlinkJunction` mutates and restores `linkPath` state within one test function that also depends on `removeReparsePoint` (a POSIX/Windows dual-purpose helper) — a short comment clarifying it's exercising the POSIX path only would help future readers, since `TestWindowsReparseRemoval` is the Windows-specific counterpart.
- `main.go`: no `//go:build` constraints or platform notes near the Gio `app.Window` setup, even though the rest of the codebase is clearly cross-platform-aware (`runtime.GOOS` branches in `link.go`) — a brief comment on supported platforms would help.

---

If you want, I can also look at `go.mod`/`go.sum` or any build tags file if those exist — they weren't in the upload set, and the `runtime.GOOS` branching plus Gio usage means platform-specific build behavior might hide additional issues I can't see from these five files alone.

Yes — there's a real bug here, and it's worse than a simple race: there's a **guaranteed self-deadlock**, plus a data race underneath it.

### 1. Deadlock: `configMu` is not reentrant, but you lock it twice

In `ui.go`, the switch-completion goroutine does:

```go
configMu.Lock()
cfg.Projects[idx].CurrentActive = t
_ = SaveConfigAtomic(cfg)
configMu.Unlock()
```

But `SaveConfigAtomic` in `config.go` *also* does:

```go
func SaveConfigAtomic(cfg *Config) error {
	configMu.Lock()
	defer configMu.Unlock()
	...
```

`sync.Mutex` in Go is not reentrant. The second `Lock()` call blocks forever waiting on a lock its own goroutine already holds. That goroutine leaks and — critically — **`configMu` stays locked permanently**. Every subsequent caller of `configMu.Lock()` (the scan coordinator's `triggerScanChan` handler, `LoadConfig`, `SaveConfigAtomic` from Add/Delete) will then also block forever. So the first successful "switch" the user performs eventually freezes all config I/O and scanning app-wide, even though the Gio render loop itself keeps running (since rendering never touches `configMu`).

Fix: don't hold `configMu` around a call to `SaveConfigAtomic` — it manages its own locking. Just do:

```go
cfg.Projects[idx].CurrentActive = t
_ = SaveConfigAtomic(cfg) // acquires its own lock
```

If you need the mutation and the save to be atomic together, either add an internal unexported `saveConfigLocked` that assumes the caller holds the lock, or protect only the field write with a separate, dedicated mutex from the file-I/O mutex.

### 2. `cfg.Projects` is only "protected" on one side

`configMu` is taken by:
- the scan coordinator goroutine (read `cfg.Projects[idx]`)
- the switch-completion goroutine (write `cfg.Projects[idx].CurrentActive`)

But it's **not** taken by the UI/main-loop goroutine, which freely does:

```go
cfg.Projects = append(cfg.Projects, p)              // AddProjectBtn handler
cfg.Projects = append(cfg.Projects[:activeIdx], cfg.Projects[activeIdx+1:]...) // DeleteBtn handler
proj := cfg.Projects[activeIdx]                      // before spawning switch goroutine
```

...and every render frame reads it unlocked too:

```go
list.Layout(gtx, len(cfg.Projects), func(gtx layout.Context, i int) ... proj := cfg.Projects[i] ...) // drawSidebar
return drawProjectDetail(gtx, th, ui, cfg.Projects[activeIdx]) // drawContent
```

A mutex only protects data if *every* accessor takes it. Here, one side locks and the other doesn't — that's still a full, `go test -race`-detectable data race. Concretely: `append` on the UI goroutine can reallocate the slice's backing array (rewriting the 3-word slice header: pointer/len/cap) at the exact moment the background goroutine is reading `len(cfg.Projects)` and indexing into it under `configMu`. That's a torn read of the slice header — possible out-of-bounds panic or reads of stale/garbage `Project` structs, not just "wrong but harmless" data.

The idiomatic fix given the rest of your architecture (which already funnels background results through `eventChan` into the single UI goroutine) is to **never mutate `cfg.Projects` from a worker goroutine at all**. Have the switch goroutine send the result back over `eventChan` (e.g. a new `Type: "SWITCH_OK"` event carrying `idx` and `t`), and let the UI goroutine — which already owns and mutates `uiState` safely this way — apply `cfg.Projects[idx].CurrentActive = t` and call `SaveConfigAtomic` itself during the `FrameEvent` drain. Then `cfg.Projects` is only ever touched by one goroutine, and `configMu` can be reserved purely for what it's named for: guarding file load/save, not in-memory slice access.

### 3. Stale index after delete

`DeleteBtn` shrinks `cfg.Projects` in place via `append(cfg.Projects[:i], cfg.Projects[i+1:]...)`, shifting every subsequent element down by one index. If a `triggerScanChan` message for some `idx > activeIdx` is still sitting in the buffered channel (capacity 20) when the delete happens, the scan coordinator will later scan whatever project *now* occupies that index — silently the wrong project — rather than erroring, since it only bounds-checks `idx < len(cfg.Projects)`.

**Priority to fix**: #1 (deadlock) will bite on essentially the first profile switch in normal use, so that's the one to fix first; #2 is the one you'd only see intermittently under `-race` or heavy use but is the deeper structural issue; #3 is a correctness edge case worth a comment/guard once the others are fixed.

Looking at `link.go`, the biggest risk is asymmetric handling between POSIX and Windows: the POSIX path builds the new link first and swaps it in atomically, but the Windows path **deletes the old link before it knows the new one can be created**. That's the core problem to fix. There are also a couple of smaller cleanup/misclassification issues that can quietly destroy user data.

Here's what I'd change, in priority order:

### 1. Windows: stop doing "remove old, then create new"

Right now:

```go
// Remove existing link / junction first (MoveFileEx replace doesn't support junctions/directories)
if err := removeReparsePoint(absLink); err != nil && !os.IsNotExist(err) {
    return fmt.Errorf("failed to clear existing link: %w", err)
}

cmd := exec.Command("cmd", "/c", "mklink", "/J", absLink, absTarget)
if output, err := cmd.CombinedOutput(); err != nil {
    errSym := os.Symlink(absTarget, absLink)
    if errSym != nil {
        return fmt.Errorf("windows junction/symlink failed: %s - %w", string(output), errSym)
    }
}
```

If `mklink` fails for *any* reason (locked by antivirus, path-length limit, non-NTFS volume, permissions, transient file-in-use error) and the `os.Symlink` fallback also fails, the old link is already gone and there's nothing to fall back to — the user's `profile` entry point simply vanishes, even though the underlying `.zen-cycle/profile_A` data is untouched. That's the "data loss" (or at least apparent data loss) risk.

Fix: build the new link at a temp name first, only touch the old one once the new one is proven to exist, and keep the old one as a recoverable backup instead of deleting it outright:

```go
func createLink(target, link string) error {
    if runtime.GOOS != "windows" {
        // POSIX path (see fix below)
    }

    absTarget, err := filepath.Abs(target)
    if err != nil {
        return err
    }
    absLink, err := filepath.Abs(link)
    if err != nil {
        return err
    }

    tmpLink := absLink + fmt.Sprintf(".tmp-%d", os.Getpid())
    _ = removeReparsePoint(tmpLink) // clear any stale leftover from a crashed run

    // 1. Create the NEW link at a temp path first — old link is untouched so far.
    cmd := exec.Command("cmd", "/c", "mklink", "/J", tmpLink, absTarget)
    if output, err := cmd.CombinedOutput(); err != nil {
        if errSym := os.Symlink(absTarget, tmpLink); errSym != nil {
            _ = removeReparsePoint(tmpLink)
            return fmt.Errorf("windows junction/symlink creation failed: %s - %w", string(output), errSym)
        }
    }

    // Sanity check the new link actually resolves before touching the old one.
    if _, err := os.Stat(tmpLink); err != nil {
        _ = removeReparsePoint(tmpLink)
        return fmt.Errorf("new link failed validation: %w", err)
    }

    // 2. Move the OLD link aside instead of deleting it, so we can restore on failure.
    backupLink := absLink + fmt.Sprintf(".bak-%d", os.Getpid())
    _ = os.Remove(backupLink)
    hadOld := false
    if _, err := os.Lstat(absLink); err == nil {
        if err := os.Rename(absLink, backupLink); err != nil {
            _ = removeReparsePoint(tmpLink)
            return fmt.Errorf("failed to move existing link aside: %w", err)
        }
        hadOld = true
    } else if !os.IsNotExist(err) {
        _ = removeReparsePoint(tmpLink)
        return err
    }

    // 3. Move the new link into place.
    if err := os.Rename(tmpLink, absLink); err != nil {
        // Restore the old link — we haven't lost anything.
        if hadOld {
            _ = os.Rename(backupLink, absLink)
        }
        _ = removeReparsePoint(tmpLink)
        return fmt.Errorf("failed to activate new link: %w", err)
    }

    // 4. Only now is it safe to discard the old link.
    if hadOld {
        _ = removeReparsePoint(backupLink)
    }
    return nil
}
```

This narrows the "nothing exists at `absLink`" window down to a single rename call (vs. the entire `mklink`/fallback attempt), and gives you an actual rollback path if that rename fails.

### 2. `removeReparsePoint` can silently delete a real, empty directory

```go
if fi.IsDir() {
    err := os.Remove(path)
    if err == nil {
        return nil
    }
    return ErrRealDirectory
}
```

This assumes: "if it's a directory and `os.Remove` succeeds, it must have been a junction." But `os.Remove` succeeds on *any* empty directory — reparse point or not. If a user manually created a plain empty folder named `profile` (before ever running the app), this silently deletes it with no warning, misreporting nothing since it returns `nil`. On modern Go, junctions are already reported via `os.ModeSymlink` on `Lstat`, so the first branch (`fi.Mode()&os.ModeSymlink != 0 → os.Remove`) already handles real junctions. That means this `IsDir` fallback should never attempt deletion at all — it should be treated as "definitely not ours, don't touch it":

```go
func removeReparsePoint(path string) error {
    fi, err := os.Lstat(path)
    if err != nil {
        return err
    }
    if fi.Mode()&os.ModeSymlink != 0 {
        return os.Remove(path) // confirmed symlink/junction
    }
    if fi.IsDir() {
        // A plain directory with no reparse attribute — never ours to delete.
        return ErrRealDirectory
    }
    return os.Remove(path)
}
```

### 3. POSIX: clean up the temp symlink on rename failure

```go
if err := os.Symlink(relTarget, tmpLink); err != nil {
    return err
}
return os.Rename(tmpLink, link)
```

The atomic-swap logic itself is sound (rename onto an existing directory fails safely on Linux, which is why `TestSymlinkJunction`'s real-directory case passes), but if `os.Rename` fails — e.g. because `link` is a real, non-empty directory — `tmpLink` is left behind as orphaned clutter next to the project. Fix:

```go
if err := os.Symlink(relTarget, tmpLink); err != nil {
    return err
}
if err := os.Rename(tmpLink, link); err != nil {
    _ = os.Remove(tmpLink)
    if fi, statErr := os.Lstat(link); statErr == nil && fi.IsDir() && fi.Mode()&os.ModeSymlink == 0 {
        return ErrRealDirectory
    }
    return err
}
return nil
```

Also worth using a less collision-prone temp name than just PID (e.g. append `time.Now().UnixNano()` too), in case of concurrent calls within the same process.

### One thing I couldn't verify without `ui.go`
I don't have `ui.go` in front of me — if the UI code persists `Project.CurrentActive` to config *before* confirming `SwitchActiveSource` succeeded, you could end up with config state claiming a switch happened when the on-disk link is still the old target (or in the Windows failure case, missing). If that ordering matters to you, send over `ui.go` and I'll check the write-config-vs-switch-link sequencing too.

I've now got all five files (ui.go included — it wasn't in the initial context bundle, so I pulled it directly). That's enough to review — no need for anything else.

Here's what would make me nervous shipping this, roughly in order of severity:

**1. The actual "cycle" behavior is never tested.**
This is the app's whole reason for existing, and `TestSymlinkJunction` only tests the very first switch — from no-link to `profile_A`. There's no test that creates `profile_A` and `profile_B`, switches to A, then switches to B, and verifies the link atomically moves and the old target's data is untouched. That path exercises `createLink`'s tmp-link-then-rename swap logic (the part actually doing the "atomic" work), and it's completely unexercised. If there's a bug in reusing/overwriting an existing symlink, this suite won't catch it.

**2. The process denylist guard — a stated safety feature — has no real coverage.**
`TestProcessRunningCheck` only checks that a process that clearly isn't running returns false. There's no test proving `SwitchActiveSource` actually *blocks* when a denylisted process is active. Since `isProcessRunning` shells out to `pgrep`/`tasklist`, it's not trivially mockable as written — which is itself worth flagging. I'd suggest extracting an injectable `processChecker` function/interface so this path can be unit tested without depending on real OS processes. Right now the "guard against split-brain writes" promise in the code comments is unverified.

**3. Corrupt config handling is untested.**
`LoadConfig` has a whole branch for corrupt JSON — it backs up the bad file to `.bak` and returns an error (`backupCorruptConfig`). Zero test writes garbage to a config file and checks (a) the `.bak` file gets created, (b) the original bytes are preserved in it, (c) the error is surfaced rather than silently swallowed.

**4. `getConfigPaths` priority order is untested.**
There's real logic here — `loadedPath` > binDir > cwd > `os.UserConfigDir()` — and none of it is verified. Worth a test that drops config files in two of these locations and confirms the higher-priority one wins.

**5. Failure/edge branches in link.go aren't hit:**
- `SwitchActiveSource` with a `newTarget` that doesn't exist in `.zen-cycle` (should error) — not tested.
- `DetectActiveSource` on a *dangling* symlink (target deleted out from under it) — `os.Readlink` succeeds but the target's gone; behavior here is untested.
- `DetectActiveSource` when the link path is a regular file, not a symlink or dir — that branch exists in code but nothing exercises it.
- `GetCycleSources` filtering of dotfiles/hidden entries — implicitly relies on there being none in the fixture, never explicitly asserted.

**6. Windows-specific code is essentially uncovered by CI.**
`TestWindowsReparseRemoval` and the junction/`mklink /J` fallback logic in `createLink` only run `t.Skip()` on non-Windows, meaning on a typical Linux CI runner this is 100% dark. Given the code explicitly special-cases Windows behavior (the `os.Remove(loadedPath)` before rename in `SaveConfigAtomic` is a good example — that actually *breaks* atomicity on Windows since a crash between remove and rename loses the file entirely), I'd want at minimum a Windows CI job actually running this, not just a local-only skip.

**7. Global mutable state (`loadedPath`, `configMu`) makes tests order-dependent.**
`cycle_test.go` reassigns the package-level `loadedPath` directly. That works today because tests happen to run in a safe order, but it's fragile — add a new test that also loads config without resetting `loadedPath` first, and you get cross-test pollution. Not a bug per se, but a maintenance trap.

**8. `handleInputs`' validation logic in ui.go is fully untested** (required fields, path-existence check, denylist CSV parsing). It's tangled into Gio widget state right now, so it's not easily unit-testable as-is — pulling the parsing/validation into a small pure function (e.g. `parseNewProject(name, path, symlink, denylistRaw string) (Project, error)`) would make this testable without touching the GUI at all, and it's a genuinely cheap refactor.

If I had to pick the one gap I'd block shipping on, it's **#1** — a tool called "cycle" that has never had its actual cycling tested end-to-end is the riskiest gap here, followed closely by #2 since that's the safety mechanism protecting user data from corruption.