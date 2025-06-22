package pretty

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"sync"
)

type groupOrAttrs struct {
	group string
	attrs []slog.Attr
}

type PrettyHandler struct {
	opts slog.HandlerOptions
	goas []groupOrAttrs
	mu   *sync.Mutex
	w    io.Writer
}

func NewHandler(w io.Writer, opts *slog.HandlerOptions) *PrettyHandler {
	h := &PrettyHandler{w: w, mu: &sync.Mutex{}}
	if opts != nil {
		h.opts = *opts
	}
	if h.opts.Level == nil {
		h.opts.Level = slog.LevelInfo
	}
	return h
}

func (h *PrettyHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.opts.Level.Level()
}

const (
	ColorReset  = "\033[0m"
	ColorMuted  = "\033[90m"
	ColorBase   = "\033[0m"
	ColorKey    = "\033[0m"
	ColorString = "\033[93m"
	ColorNumber = "\033[92m"
	ColorBool   = "\033[94m"
	ColorNull   = "\033[95m"

	ColorDebug = "\033[37m"
	ColorInfo  = "\033[94m"
	ColorWarn  = "\033[33m"
	ColorError = "\033[31m"
)

func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	buf := make([]byte, 0, 1024)
	if !r.Time.IsZero() {
		buf = fmt.Appendf(buf, "%s[%s]%s", ColorMuted, r.Time.Format("15:04:05.000"), ColorReset)
	}

	switch r.Level {
	case slog.LevelDebug:
		buf = fmt.Appendf(buf, " %s%s%s:", ColorDebug, r.Level, ColorMuted)
	case slog.LevelInfo:
		buf = fmt.Appendf(buf, " %s%s%s:", ColorInfo, r.Level, ColorMuted)
	case slog.LevelWarn:
		buf = fmt.Appendf(buf, " %s%s%s:", ColorWarn, r.Level, ColorMuted)
	case slog.LevelError:
		buf = fmt.Appendf(buf, " %s%s%s:", ColorError, r.Level, ColorMuted)
	default:
		buf = fmt.Appendf(buf, " %s%s:", r.Level, ColorMuted)
	}

	buf = fmt.Appendf(buf, " %s%s%s", ColorBase, r.Message, ColorMuted)
	if r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		buf = fmt.Appendf(buf, " %s:%d", f.File, f.Line)
	}

	goas := h.goas
	if r.NumAttrs() == 0 {
		// If the record has no attrs, remove groups at the end of the list
		// (since they're empty)
		for len(goas) > 0 && goas[len(goas)-1].group != "" {
			goas = goas[:len(goas)-1]
		}
	}

	if len(goas)+r.NumAttrs() > 0 {
		buf = fmt.Append(buf, " {")

		indentLevel := 1
		firstProp := true
		for _, goa := range goas {
			if goa.group != "" {
				if !firstProp {
					buf = fmt.Append(buf, ",")
				}
				buf = fmt.Appendf(buf, "\n%*s%s%q%s: {", indentLevel*2, "", ColorKey, goa.group, ColorMuted)
				indentLevel++
				firstProp = true
				for _, a := range goa.attrs {
					buf, firstProp = h.appendAttr(buf, a, indentLevel, firstProp)
				}
			} else {
				for _, a := range goa.attrs {
					buf, firstProp = h.appendAttr(buf, a, indentLevel, firstProp)
				}
			}
		}
		r.Attrs(func(a slog.Attr) bool {
			buf, firstProp = h.appendAttr(buf, a, indentLevel, firstProp)
			return true
		})

		for indentLevel > 0 {
			indentLevel--
			buf = fmt.Appendf(buf, "\n%*s}", indentLevel*2, "")
		}
	}

	buf = fmt.Appendf(buf, "%s\n", ColorReset)

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(buf)
	return err
}

func (h *PrettyHandler) appendAttr(buf []byte, a slog.Attr, indentLevel int, firstProp bool) ([]byte, bool) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return buf, firstProp
	}

	if !firstProp {
		buf = fmt.Append(buf, ",")
	}
	buf = fmt.Appendf(buf, "\n%*s%s%q%s: ", indentLevel*2, "", ColorKey, a.Key, ColorMuted)

	switch a.Value.Kind() {
	case slog.KindGroup:
		attrs := a.Value.Group()
		if len(attrs) == 0 {
			buf = fmt.Append(buf, "{}")
			return buf, false
		}

		buf = fmt.Append(buf, "{")
		nestedFirstProp := true
		nestedIndentLevel := indentLevel + 1
		for _, ga := range attrs {
			buf, nestedFirstProp = h.appendAttr(buf, ga, nestedIndentLevel, nestedFirstProp)
		}
		buf = fmt.Appendf(buf, "\n%*s}", indentLevel*2, "")
		return buf, false

	default:
		var val any
		switch a.Value.Kind() {
		case slog.KindString:
			buf = fmt.Append(buf, ColorString)
			val = a.Value.String()
		case slog.KindInt64:
			buf = fmt.Append(buf, ColorNumber)
			val = a.Value.Int64()
		case slog.KindUint64:
			buf = fmt.Append(buf, ColorNumber)
			val = a.Value.Uint64()
		case slog.KindFloat64:
			buf = fmt.Append(buf, ColorNumber)
			val = a.Value.Float64()
		case slog.KindBool:
			buf = fmt.Append(buf, ColorBool)
			val = a.Value.Bool()
		case slog.KindDuration:
			buf = fmt.Append(buf, ColorString)
			val = a.Value.Duration().String()
		case slog.KindTime:
			buf = fmt.Append(buf, ColorString)
			val = a.Value.Time().Format("2006-01-02T15:04:05.000Z07:00")
		case slog.KindAny:
			if a.Value.Any() == nil {
				buf = fmt.Append(buf, ColorNull)
				val = a.Value.Any()
			} else {
				buf = fmt.Append(buf, ColorString)
				val = a.Value.String()
			}
		default:
			buf = fmt.Append(buf, ColorString)
			val = a.Value.String()
		}

		encodedVal, err := json.Marshal(val)
		if err != nil {
			encodedVal = fmt.Appendf(nil, "%q", fmt.Sprintf("<error marshalling: %v>", err))
		}
		buf = fmt.Appendf(buf, "%s%s", encodedVal, ColorMuted)
	}

	return buf, false
}

func (h *PrettyHandler) withGroupOrAttrs(goa groupOrAttrs) *PrettyHandler {
	h2 := *h
	h2.goas = make([]groupOrAttrs, len(h.goas)+1)
	copy(h2.goas, h.goas)
	h2.goas[len(h2.goas)-1] = goa
	return &h2
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return h.withGroupOrAttrs(groupOrAttrs{attrs: attrs})
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.withGroupOrAttrs(groupOrAttrs{group: name})
}
