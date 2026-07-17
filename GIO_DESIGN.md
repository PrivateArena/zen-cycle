# Gio UI Design Notes for zen-cycle

## Scroll Speed Multiplier (`layout.List`)

The profile list uses a `layout.List` inside a scrollable sidebar. To multiply
scroll speed without cascading or clamp bounce:

**Pattern:** `ScrollBy` + settling debounce + boundary cap.

```go
type UIState struct {
    ScrollSettling bool
}

// In the Flexed callback containing the list:

if ui.ScrollSettling {
    ui.ScrollSettling = false
    return listLayout(gtx)
}

prevFirst := ui.ProfileList.Position.First
prevOffset := ui.ProfileList.Position.Offset
dims := listLayout(gtx)

m := scrollSpeed // configurable multiplier, 1.0 = default
if m > 0 && m != 1.0 {
    n := len(ui.AvailableProfiles)
    pos := ui.ProfileList.Position
    if n > 0 && pos.Length > 0 {
        avg := float64(pos.Length) / float64(n)
        d := float64(pos.First-prevFirst)*avg + float64(pos.Offset-prevOffset)
        if d > 0.5 || d < -0.5 {
            extra := d * (m - 1.0)
            // Cap at remaining scrollable distance to avoid clamp bounce
            if d > 0 {
                rem := float64(n-pos.First)*avg - float64(pos.Offset)
                if extra > rem {
                    extra = rem
                }
            } else {
                avail := float64(pos.First)*avg + float64(pos.Offset)
                if -extra > avail {
                    extra = -avail
                }
            }
            items := float32(extra / avg)
            if items != 0 {
                ui.ProfileList.ScrollBy(items)
                ui.ScrollSettling = true
                gtx.Execute(op.InvalidateCmd{})
            }
        }
    }
}
```

**Why not alternatives:**
- `ScrollBy` without debounce → cascading: the ScrollBy modifies Position,
  next frame detects that change as a "user scroll" and applies ScrollBy again
  → infinite loop / ghosting.
- Post-Layout direct `Position.Offset` modification → one-frame delay, and
  pushing past content bounds triggers Gio's internal `nextDir()` clamping
  which creates a bounce-back oscillation.
- Pre-Layout accumulated offset → same clamp bounce issue.
- Pointer event interception → Gio's area hierarchy makes it unreliable;
  a handler at the Flexed level does not consistently receive scroll events
  that the list's internal `gesture.Scroll` processes.

## Toast/Status Overlays

Use `layout.Stack{Expanded, Stacked}` — never `layout.Flex` with a status bar
rigid — so status messages overlay content without pushing it down.

```go
layout.Stack{}.Layout(gtx,
    layout.Expanded(func(gtx C) D {
        return drawContent(gtx, ...)
    }),
    layout.Stacked(func(gtx C) D {
        if msg != "" {
            return layout.Inset{...}.Layout(gtx, func(gtx C) D {
                return drawAlert(gtx, ...)
            })
        }
        return D{}
    }),
)
```

`layout.Stacked` positions at the top-left of the Stack — use `layout.Inset`
to place it where desired.

## List Scroll Position Persistence

Store `layout.List` **inside UIState** — never create a new one per frame.
Otherwise scroll Position resets every frame.

```go
type UIState struct {
    ProfileList layout.List // persists scroll Position
}
```

Then reuse it in every frame:
```go
dims := ui.ProfileList.Layout(gtx, len(items), func(gtx C, i int) D {
    return renderItem(gtx, items[i])
})
```

## Background Fill for List Items

Use `paint.FillShape` with `clip.Rect{Max: gtx.Constraints.Min}` inside a
`layout.Expanded` child. Do NOT use `clip.Rect{Max: gtx.Constraints.Min}` at
the outer scope — on the first frame `Constraints.Min` is (0,0) and clips to
nothing.

```go
layout.Stack{}.Layout(gtx,
    layout.Expanded(func(gtx C) D {
        paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Min}.Op())
        return D{Size: gtx.Constraints.Min}
    }),
    layout.Stacked(func(gtx C) D {
        return layout.Inset{...}.Layout(gtx, func(gtx C) D {
            return label.Layout(gtx)
        })
    }),
)
```

## Compact Layout Heuristics

- Use `unit.Dp(8-12)` for spacers/insets instead of the default `16-24`.
- Use `material.Body2` instead of `Body1` for profile names.
- Reduce item padding: `Inset{Top: 6, Bottom: 6, Left: 12, Right: 12}`.
- `layout.Spacer{Height: 4-8}` between sections.

## Config Pattern

Store user-facing settings directly in the JSON-backed `Config` struct.
Validate on load with zero-value guards:

```go
type Config struct {
    ScrollSpeed float64 `json:"scroll_speed"`
}

func LoadConfig() *Config {
    // ...
    if cfg.ScrollSpeed <= 0 {
        cfg.ScrollSpeed = 1.0
    }
    return &cfg
}
```

Avoid embedding config in UIState; pass it as a parameter to draw functions.
