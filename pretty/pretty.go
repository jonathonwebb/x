package pretty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/jonathonwebb/x/color"
)

type PrettyHandlerOptions struct {
	TimeFormat string
	SingleLine bool
	NoColor    bool
}

type PrettyHandler struct {
	slog.Handler
	buf  *bytes.Buffer
	mu   *sync.Mutex
	w    io.Writer
	opts PrettyHandlerOptions
}

func NewPrettyHandler(w io.Writer, opts *slog.HandlerOptions, prettyOpts *PrettyHandlerOptions) *PrettyHandler {
	defaultOpts := PrettyHandlerOptions{
		TimeFormat: "2006-01-02 15:04:05",
	}

	finalOpts := defaultOpts
	if prettyOpts != nil {
		if prettyOpts.TimeFormat != "" {
			finalOpts.TimeFormat = prettyOpts.TimeFormat
		}
		finalOpts.NoColor = prettyOpts.NoColor
		finalOpts.SingleLine = prettyOpts.SingleLine
	}

	h := &PrettyHandler{
		buf:  &bytes.Buffer{},
		mu:   &sync.Mutex{},
		w:    w,
		opts: finalOpts,
	}
	jsonOpts := *opts
	jsonOpts.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey || a.Key == slog.LevelKey || a.Key == slog.MessageKey || a.Key == slog.SourceKey {
			return slog.Attr{} // exclude from output
		}
		if opts.ReplaceAttr != nil {
			return opts.ReplaceAttr(groups, a)
		}
		return a
	}
	h.Handler = slog.NewJSONHandler(h.buf, &jsonOpts)
	return h
}

func (h *PrettyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Handler.Enabled(ctx, level)
}

func (h *PrettyHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	defer h.buf.Reset()

	if err := h.Handler.Handle(ctx, r); err != nil {
		return err
	}

	timeStr := r.Time.Format(h.opts.TimeFormat)

	rawJSON := h.buf.Bytes()
	if len(rawJSON) == 0 {
		rawJSON = []byte("{}")
	}

	var data map[string]any
	if err := json.Unmarshal(rawJSON, &data); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	var formattedJSON []byte
	var err error
	if h.opts.SingleLine {
		formattedJSON, err = json.Marshal(data)
	} else {
		formattedJSON, err = json.MarshalIndent(data, "", "  ")
	}

	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	var levelColor, msgColor, jsonColor color.Color
	if !h.opts.NoColor {
		switch r.Level {
		case slog.LevelDebug:
			levelColor = color.Gray
		case slog.LevelInfo:
			levelColor = color.Blue
		case slog.LevelWarn:
			levelColor = color.Yellow
		case slog.LevelError:
			levelColor = color.Red
		default:
			levelColor = color.Magenta
		}

		msgColor = color.Cyan
		jsonColor = color.Gray
	}

	line := fmt.Sprintf("[%s] %s: %s %s\n",
		timeStr,
		color.Colorf(levelColor, "%s", r.Level.String()),
		color.Colorf(msgColor, "%s", r.Message),
		color.Colorf(jsonColor, "%s", string(formattedJSON)),
	)

	_, err = h.w.Write([]byte(line))
	return err
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &PrettyHandler{
		Handler: h.Handler.WithAttrs(attrs),
		buf:     h.buf, // share the same buffer
		mu:      h.mu,  // share the same writer mutex
		w:       h.w,
		opts:    h.opts,
	}
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	return &PrettyHandler{
		Handler: h.Handler.WithGroup(name),
		buf:     h.buf, // share the same buffer
		mu:      h.mu,  // share the same writer mutex
		w:       h.w,
		opts:    h.opts,
	}
}
