package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	BOLD    = 1
	NO_BOLD = 22

	UNDERLINE    = 4
	NO_UNDERLINE = 24

	NEGATIVE = 7
	POSITIVE = 27

	FG_BLACK   = 30
	FG_RED     = 31
	FG_GREEN   = 32
	FG_YELLOW  = 33
	FG_BLUE    = 34
	FG_MAGENTA = 35
	FG_CYAN    = 36
	FG_WHITE   = 37
	FG_EXT     = 38
	FG_DEFAULT = 39

	BG_BLACK   = 40
	BG_RED     = 41
	BG_GREEN   = 42
	BG_YELLOW  = 43
	BG_BLUE    = 44
	BG_MAGENTA = 45
	BG_CYAN    = 46
	BG_WHITE   = 47
	BG_EXT     = 48
	BG_DEFAULT = 49

	FG_BLACK_B   = 90
	FG_RED_B     = 91
	FG_GREEN_B   = 92
	FG_YELLOW_B  = 93
	FG_BLUE_B    = 94
	FG_MAGENTA_B = 95
	FG_CYAN_B    = 96
	FG_WHITE_B   = 97

	BG_BLACK_B   = 100
	BG_RED_B     = 101
	BG_GREEN_B   = 102
	BG_YELLOW_B  = 103
	BG_BLUE_B    = 104
	BG_MAGENTA_B = 105
	BG_CYAN_B    = 106
	BG_WHITE_B   = 107
)

func FG_RGB(r int, g int, b int) (int, int, int, int, int) {
	return 38, 2, r, g, b
}

func BG_RGB(r int, g int, b int) (int, int, int, int, int) {
	return 48, 2, r, g, b
}

const (
	ESC      = '\x1b'
	BOX_S_H  = '─'
	BOX_S_V  = '│'
	BOX_S_TL = '┌'
	BOX_S_TR = '┐'
	BOX_S_BL = '└'
	BOX_S_BR = '┘'
)

type Box struct {
	x uint
	y uint
	w uint
	h uint
}

func boxesCollide(box1 Box, box2 Box) bool {
	return !(box2.x+box2.w <= box1.x || box1.x+box1.w <= box2.x || box2.y+box2.h <= box1.y || box1.y+box1.h <= box2.y)
}

type Com string
type ComBuilder struct {
	offX uint
	offY uint

	win     *Window
	builder *strings.Builder
}

func NewCom() *ComBuilder { return &ComBuilder{} }

func (cb *ComBuilder) MoveTo(x uint, y uint) *ComBuilder {
	cb.builder.WriteString(fmt.Sprintf("%c[%d;%dH", ESC, cb.offY+y, cb.offX+x))
	return cb
}

func (cb *ComBuilder) Offset(x int, y int) *ComBuilder {
	if x > 0 {
		cb.builder.WriteString(fmt.Sprintf("%c[%dC", ESC, x))
	} else if x < 0 {
		cb.builder.WriteString(fmt.Sprintf("%c[%dD", ESC, -x))
	}

	if y > 0 {
		cb.builder.WriteString(fmt.Sprintf("%c[%dB", ESC, y))
	} else if y < 0 {
		cb.builder.WriteString(fmt.Sprintf("%c[%dA", ESC, -y))
	}

	return cb
}

func (cb *ComBuilder) Write(text ...any) *ComBuilder {
	for _, t := range text {
		switch t := t.(type) {
		case string:
			cb.builder.WriteString(t)
		case rune:
			cb.builder.WriteRune(t)
		case byte:
			cb.builder.WriteByte(t)
		default:
			cb.builder.WriteString(fmt.Sprintf("%v", t))
		}
	}

	return cb
}

func (cb *ComBuilder) MoveLines(lines int) *ComBuilder {
	if lines > 0 {
		cb.builder.WriteString(fmt.Sprintf("%c[%dE", ESC, lines))
		cb.Offset(int(cb.offX), 0)
	} else if lines < 0 {
		cb.builder.WriteString(fmt.Sprintf("%c[%dF", ESC, -lines))
		cb.Offset(int(cb.offX), 0)
	}
	return cb
}

func (cb *ComBuilder) Clear() *ComBuilder {
	cb.builder.WriteRune(ESC)
	cb.builder.WriteString("[2J")
	return cb
}

func (cb *ComBuilder) BuildCom() Com {
	return Com(cb.builder.String())
}

func (cb *ComBuilder) DrawBox(box Box, title string) *ComBuilder {
	//ensure width fits title
	if len(title) > int(box.w)-2 || box.h < 2 {
		return cb
	}

	//offset
	cb.Offset(int(box.x), int(box.y)).Write(BOX_S_TL, title, strings.Repeat(string(BOX_S_H), int(box.w)-2-len(title)), BOX_S_TR)
	//write
	for i := 0; i < int(box.h)-2; i++ {
		cb.MoveLines(1).Offset(int(box.x), 0).Write(BOX_S_V).Offset(int(box.w)-2, 0).Write(BOX_S_V)
	}

	return cb.MoveLines(1).Offset(int(box.x), 0).Write(BOX_S_BL, strings.Repeat(string(BOX_S_H), int(box.w)-2), BOX_S_BR)
}

func (cb *ComBuilder) SelectGraphicsRendition(formatOptions ...int) *ComBuilder {
	if len(formatOptions) < 1 {
		return cb
	}
	cb.builder.WriteString(fmt.Sprintf("%c[%d", ESC, formatOptions[0]))
	for opt := range formatOptions[1:] {
		cb.builder.WriteString(fmt.Sprintf(";%d", opt))
	}
	cb.builder.WriteByte('m')
	return cb
}

func (cb *ComBuilder) ClearGraphicsRendition() *ComBuilder {
	cb.builder.WriteRune(ESC)
	cb.builder.WriteString("[0m")
	return cb
}

func (cb *ComBuilder) Exec() {
	// sends command to window
	cb.win.Exec(cb.BuildCom())
}

type Window struct {
	parent   *Window
	children []*Window
	Box

	coms chan Com
}

func (win *Window) GetDimensions() (w int, h int) {
	return int(win.w), int(win.h)
}

func (win *Window) WithinBounds(box Box) bool {
	//check if top left corner is within bounds
	inTopL := box.x < win.w && box.y < win.h
	//check if bottom right corner is within bounds
	inBotR := box.x+box.w <= win.w && box.y+box.h <= win.h

	return inTopL && inBotR

}

func (win *Window) NewChild(box Box) (child *Window) {
	if !win.WithinBounds(box) {
		return nil
	}

	//check if collides
	for _, child := range win.children {
		if boxesCollide(box, child.Box) {
			return nil
		}
	}

	child = &Window{win, []*Window{}, box, win.coms}
	win.children = append(win.children, child)
	return child
}

func (win *Window) GetOffsetComBuilder() *ComBuilder {
	var cb *ComBuilder
	if win.parent != nil {
		cb = win.parent.GetOffsetComBuilder()
		//permanent offset for dealing with relative coordinates
		cb.offX += win.x
		cb.offY += win.y

		cb.win = win
		//starting offset
		cb.Offset(int(win.x), int(win.y))
	} else {
		cb = &ComBuilder{0, 0, win, &strings.Builder{}}
		cb.MoveTo(0, 0)
	}

	return cb
}

func (win *Window) Exec(com Com) {
	win.coms <- com
}

func InitTerminalLoop() (root *Window, loop func(), userInput chan byte) {
	old, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Println(err)
		return
	}

	//define root window
	w, h := GetDimensions()

	commands := make(chan Com, 256)
	root = &Window{nil, []*Window{}, Box{1, 1, uint(w), uint(h)}, commands}
	userInput = make(chan byte)
	return root, startTerminalLoop(commands, userInput, old), userInput
}

func startTerminalLoop(commands chan Com, userIn chan byte, oldState *term.State) func() {
	return func() {
		defer endTerminalLoop(oldState)

		inChan := make(chan byte, 8)
		go inputLoop(inChan)
		// hide cursor
		fmt.Printf("%c[?25l", ESC)

		for {
			select {
			case command := <-commands:
				fmt.Print(command)
			case in := <-inChan:
				if in == KEY_QUIT {
					return
				} else {
					userIn <- in
				}
			}

		}
	}
}

func endTerminalLoop(oldState *term.State) {
	//clear screen
	fmt.Printf("%c[2J", ESC)
	//move to 1,1
	fmt.Printf("%c[1;1H", ESC)
	//reset
	fmt.Printf("%c[!p", ESC)
	//restore
	term.Restore(int(os.Stdin.Fd()), oldState)
}

func inputLoop(input chan byte) {
	char := make([]byte, 1)
	for {
		count, _ := os.Stdin.Read(char)
		if count > 0 {
			input <- char[0]
		}
	}
}

func GetDimensions() (w int, h int) {
	fmt.Print("\x1b[2J\x1b[999C\x1b[999B")
	fmt.Print("\x1b[6n")
	input := ReadTo('R')
	fmt.Sscanf(input, "\x1b[%d;%dR", &h, &w)
	fmt.Print("\x1b[0;0H")
	return w, h
}

func ReadTo(b byte) string {
	curByte := make([]byte, 1)
	os.Stdin.Read(curByte)
	outArr := make([]byte, 0)
	outArr = append(outArr, curByte...)
	for curByte[0] != b {
		os.Stdin.Read(curByte)
		outArr = append(outArr, curByte...)
	}
	return string(outArr[:])
}
