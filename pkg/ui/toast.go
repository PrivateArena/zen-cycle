package ui

import (
	"image/color"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// ToastType represents the category of a toast notification.
type ToastType string

const (
	ToastError   ToastType = "ERROR"
	ToastSuccess ToastType = "SUCCESS"
	ToastInfo    ToastType = "INFO"
)

// Toast represents a transient notification with an auto-dismiss deadline.
type Toast struct {
	Message   string
	Type      ToastType
	CreatedAt time.Time
	Duration  time.Duration
}

// IsExpired reports whether the toast has exceeded its display duration.
func (t Toast) IsExpired() bool {
	return time.Since(t.CreatedAt) > t.Duration
}

// ToastQueue manages a thread-safe queue of active toasts.
type ToastQueue struct {
	mu     sync.Mutex
	toasts []Toast
}

// Push appends a toast to the queue with the given duration.
func (q *ToastQueue) Push(msg string, t ToastType, dur time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.toasts = append(q.toasts, Toast{
		Message:   msg,
		Type:      t,
		CreatedAt: time.Now(),
		Duration:  dur,
	})
}

// DrainExpired removes expired toasts and returns them.
func (q *ToastQueue) DrainExpired() []Toast {
	q.mu.Lock()
	defer q.mu.Unlock()
	var expired []Toast
	keep := q.toasts[:0]
	for _, t := range q.toasts {
		if t.IsExpired() {
			expired = append(expired, t)
		} else {
			keep = append(keep, t)
		}
	}
	q.toasts = keep
	return expired
}

// Len returns the number of active (non-expired) toasts.
func (q *ToastQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.toasts)
}

// Clear removes all active toasts.
func (q *ToastQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.toasts = nil
}

// ToastDefaultDuration is the default auto-dismiss interval for toasts.
const ToastDefaultDuration = 4 * time.Second

// toastColor maps toast types to their display color.
func toastColor(t ToastType) color.NRGBA {
	switch t {
	case ToastError:
		return colorRed
	case ToastSuccess:
		return colorGreen
	default:
		return colorAccent
	}
}

// DrawToast renders a single toast notification as a horizontal bar.
func DrawToast(gtx layout.Context, th *material.Theme, t Toast) layout.Dimensions {
	bg := toastColor(t.Type)
	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, color.NRGBA{R: bg.R, G: bg.G, B: bg.B, A: 0x22}, clip.Rect{Max: gtx.Constraints.Min}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(th, t.Message)
					lbl.Color = bg
					return lbl.Layout(gtx)
				})
			}),
		)
	})
}

// DrawToastOverlay renders all active toasts as a floating overlay in the content area.
func DrawToastOverlay(gtx layout.Context, th *material.Theme, q *ToastQueue) layout.Dimensions {
	q.mu.Lock()
	toasts := make([]Toast, len(q.toasts))
	copy(toasts, q.toasts)
	q.mu.Unlock()

	if len(toasts) == 0 {
		return layout.Dimensions{}
	}

	dims := layout.Dimensions{}
	for _, t := range toasts {
		dims = layout.Inset{Top: unit.Dp(8), Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return DrawToast(gtx, th, t)
		})
	}
	return dims
}
