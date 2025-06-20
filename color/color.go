package color

import "fmt"

type Color string

const (
	Reset   Color = "0"
	Red     Color = "31"
	Green   Color = "32"
	Yellow  Color = "33"
	Blue    Color = "34"
	Magenta Color = "35"
	Cyan    Color = "36"
	Gray    Color = "37"
)

func (c Color) Escape() string {
	return fmt.Sprintf("\033[%sm", c)
}

func Colorf(color Color, format string, args ...any) string {
	if color == "" {
		return fmt.Sprintf(format, args...)
	}
	return fmt.Sprintf("%s%s%s", color.Escape(), fmt.Sprintf(format, args...), Reset.Escape())
}
