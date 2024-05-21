package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

const (
	ESC      = '\x1b'
	BOX_S_H  = '─'
	BOX_S_V  = '│'
	BOX_S_TL = '┌'
	BOX_S_TR = '┐'
	BOX_S_BL = '└'
	BOX_S_BR = '┘'
)

type Com string
type ComBuilder struct {
	offX    uint
	offY    uint
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
	} else if lines < 0 {
		cb.builder.WriteString(fmt.Sprintf("%c[%dF", ESC, -lines))
	}
	cb.Offset(int(cb.offX), 0)
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

type Box struct {
	x uint
	y uint
	w uint
	h uint
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

type Window struct {
	parent   *Window
	children []*Window
	Box
}

func InitTerminalLoop(freq int, commands chan Com) (root *Window, loop func()) {
	old, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Println(err)
		return
	}

	//define root window
	w, h := GetDimensions()
	root = &Window{nil, []*Window{}, Box{1, 1, uint(w), uint(h)}}

	return root, startTerminalLoop(freq, commands, old)
}

func startTerminalLoop(freq int, commands chan Com, oldState *term.State) func() {
	return func() {
		defer endTerminalLoop(oldState)
		clock := time.NewTicker(time.Second / time.Duration(freq))

		inChan := make(chan byte, 8)
		go inputLoop(inChan)
		// hide cursor
		fmt.Printf("%c[?25l", ESC)

		for {
			<-clock.C
			select {
			case command := <-commands:
				fmt.Print(command)
			case in := <-inChan:
				if in == 'q' {
					return
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

func (win *Window) WithinBounds(box Box) bool {
	//check if top left corner is within bounds
	inTopL := box.x < win.w && box.y < win.h
	//check if bottom right corner is within bounds
	inBotR := box.x+box.w <= win.w && box.y+box.h <= win.h

	return inTopL && inBotR

}

func boxesCollide(box1 Box, box2 Box) bool {
	return !(box2.x+box2.w <= box1.x || box1.x+box1.w <= box2.x || box2.y+box2.h <= box1.y || box1.y+box1.h <= box2.y)
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

	child = &Window{win, []*Window{}, box}
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
		//starting offset
		cb.Offset(int(win.x), int(win.x))
	} else {
		cb = &ComBuilder{0, 0, &strings.Builder{}}
		cb.MoveTo(0, 0)
	}

	return cb
}

func (win *Window) DrawBox(box Box, title string) Com {
	if !win.WithinBounds(box) {
		return ""
	}

	return win.GetOffsetComBuilder().DrawBox(box, title).BuildCom()
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
