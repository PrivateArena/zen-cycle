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

## WHY GIO OVER FYNE ?
This image showing absolute 0% CPU 

[Image](data:image/png;base64,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)
