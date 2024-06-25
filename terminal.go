package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

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

type FloatBox struct {
	x float64
	y float64
	w float64
	h float64
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
	w, h := cb.win.GetDimensions()
	clearString := strings.Repeat(" ", w)
	cb.MoveTo(1, 1)
	for i := 0; i < h; i++ {
		cb.builder.WriteString(clearString)
		cb.MoveLines(1)
	}
	cb.MoveTo(1, 1)
	return cb
}

func (cb *ComBuilder) BuildCom() Com {
	return Com(cb.builder.String())
}

func (cb *ComBuilder) DrawBox(box Box, title string, highlighted bool) *ComBuilder {
	//ensure width fits title
	if len(title) > int(box.w)-2 || box.h < 2 {
		return cb
	}

	graphicsMode := POSITIVE
	if highlighted {
		graphicsMode = NEGATIVE
	}

	//offset
	cb.Offset(int(box.x), int(box.y)).Write(BOX_S_TL).SelectGraphicsRendition(graphicsMode).Write(title).ClearGraphicsRendition().Write(strings.Repeat(string(BOX_S_H), int(box.w)-2-len(title)), BOX_S_TR)

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
	parent         *Window
	children       []*Window
	childPositions []FloatBox
	Box

	coms chan Com
	con  Controller

	selectable bool
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

func (win *Window) NewChild(box Box, selectable bool) (child *Window) {
	if !win.WithinBounds(box) {
		return nil
	}

	//check if collides
	for _, child := range win.children {
		if boxesCollide(box, child.Box) {
			return nil
		}
	}

	child = &Window{win, []*Window{}, []FloatBox{}, box, win.coms, nil, selectable}
	win.children = append(win.children, child)

	// calculate relative dimensions
	relX := float64(box.x) / float64(win.w)
	relY := float64(box.y) / float64(win.h)
	relW := float64(box.w) / float64(win.w)
	relH := float64(box.h) / float64(win.h)
	win.childPositions = append(win.childPositions, FloatBox{relX, relY, relW, relH})
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

func (win *Window) Resize(b Box) {
	win.Box = b

	if win.con != nil {
		win.con.Resize()
	}
	for i, w := range win.children {
		newX := uint(math.Round(float64(win.w) * win.childPositions[i].x))
		newY := uint(math.Round(float64(win.h) * win.childPositions[i].y))
		newW := uint(math.Round(float64(win.w) * win.childPositions[i].w))
		newH := uint(math.Round(float64(win.h) * win.childPositions[i].h))
		w.Resize(Box{newX, newY, newW, newH})
	}

}

func (win *Window) Exec(com Com) {
	win.coms <- com
}

func (win *Window) SetController(c func(*Window) Controller) {
	win.con = c(win)
}

func (win *Window) NewInnerChild(levels int, canSelect bool) (child *Window) {
	childBox := Box{uint(levels), uint(levels), win.w - uint(2*levels), win.h - uint(2*levels)}
	child = win.NewChild(childBox, canSelect)
	return child
}

func (win *Window) HSplit() (sibling *Window) {
	halfW := win.w / 2
	// create parent
	var parent *Window
	if win.parent == nil {
		parent = &Window{win.parent, []*Window{win}, []FloatBox{{0, 0, float64(halfW) / float64(win.w), 1}}, win.Box, win.coms, nil, false}
		win.x, win.y = 0, 0
	} else {
		var err error
		parent, err = win.parent.createIntermediateChild(win)
		if err != nil {
			return nil
		}
		parent.childPositions[0] = FloatBox{0, 0, float64(halfW) / float64(win.w), 1}
	}

	win.parent = parent

	// shrink
	win.w = halfW

	// create sibling
	sibBox := Box{halfW, 0, parent.w - halfW, win.h}

	sibling = parent.NewChild(sibBox, true)

	return sibling
}

func (win *Window) VSplit() (sibling *Window) {
	halfH := win.h / 2

	// create parent
	var parent *Window
	if win.parent == nil {
		parent = &Window{win.parent, []*Window{win}, []FloatBox{{0, 0, 1, float64(halfH) / float64(win.h)}}, win.Box, win.coms, nil, false}
		win.x, win.y = 0, 0
	} else {
		var err error
		parent, err = win.parent.createIntermediateChild(win)
		if err != nil {
			return nil
		}
		parent.childPositions[0] = FloatBox{0, 0, 1, float64(halfH) / float64(win.h)}
	}

	win.parent = parent

	// shrink
	win.h = halfH

	// create sibling
	sibBox := Box{0, halfH, win.w, parent.h - halfH}

	sibling = parent.NewChild(sibBox, true)

	return sibling
}

func (win *Window) createIntermediateChild(old *Window) (new *Window, err error) {

	for i, child := range win.children {
		if old == child {
			new = &Window{win, []*Window{old}, []FloatBox{{0, 0, 1, 1}}, old.Box, old.coms, nil, false}
			win.children[i] = new
			return new, nil
		}
	}

	return nil, errors.New("could not find old window")
}
func (win *Window) GetRoot() *Window {
	if win.parent == nil {
		return win
	} else {
		return win.parent.GetRoot()
	}
}

func (win *Window) Select() {
	if win.con != nil {
		win.con.Select()
	}
}

func (win *Window) Deselect() {
	if win.con != nil {
		win.con.Deselect()
	}
}

func (win *Window) ResolveInput(b byte) bool {
	if win.con == nil || !win.con.ResolveInput(b) {
		if win.parent != nil {
			return win.parent.ResolveInput(b)
		} else {
			return false
		}
	}
	return true
}

type Controller interface {
	Select()
	Deselect()
	Resize()

	ResolveInput(byte) bool
}

type WindowVisitor struct {
	cur     *Window
	history []int
}

func NewWindowVisitor(win *Window) *WindowVisitor {
	return &WindowVisitor{win, []int{-1}}
}

func (v *WindowVisitor) Current() *Window {
	return v.cur
}

func (v *WindowVisitor) Next() *Window {
	// increment latest idx
	last := len(v.history) - 1
	v.history[last]++

	// check if valid child exists
	if len(v.cur.children) > v.history[last] {
		// get child
		v.cur = v.cur.children[v.history[last]]
	} else {
		// go up one layer in history
		v.history = v.history[:last]
		if len(v.history) == 0 { // root changed since initialization. add layer to history
			v.history = []int{0}
		}

		// move to parent if exists
		if v.cur.parent != nil {
			v.cur = v.cur.parent
			return v.Next()
		}
	}
	v.history = append(v.history, -1)
	if v.cur.selectable {
		return v.cur
	} else {
		return v.Next()
	}
}

func InitTerminalLoop() (root *Window, quit chan struct{}, globalInput chan byte) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Println(err)
		return
	}
	inChan := make(chan byte, 8)
	go inputLoop(inChan)

	quitChan := make(chan struct{})
	commands := make(chan Com, 8)
	dimensions := make(chan int)
	leftover := make(chan byte, 8)

	//define root window

	root = &Window{nil, []*Window{}, []FloatBox{}, Box{1, 1, 0, 0}, commands, nil, true}
	go terminalLoop(oldState, root, commands, inChan, leftover, dimensions, quitChan)

	GetDimensions := GetWindowDimensionsFunc(commands, dimensions)
	w, h := GetDimensions()
	root.Resize(Box{1, 1, uint(w), uint(h)})

	//resize loop
	go func() {
		curW, curH := GetDimensions()
		resizeClock := time.NewTicker(100 * time.Millisecond)

		for {
			<-resizeClock.C
			w, h := GetDimensions()

			if w != curW || h != curH {
				commands <- "\x1b[2J\x1b[3J\x1b[H"
				curW, curH = w, h
				root.GetRoot().Resize(Box{1, 1, uint(curW), uint(curH)})
			}
		}
	}()
	root.GetOffsetComBuilder().Clear().Exec()
	return root, quitChan, leftover
}

func terminalLoop(oldState *term.State, root *Window, commandChan <-chan Com, userInputChan <-chan byte, leftoverInput chan<- byte, dimensionsChan chan<- int, quitChan chan<- struct{}) {

	go func() {
		defer endTerminalLoop(oldState)

		// hide cursor
		fmt.Printf("%c[?25l", ESC)

		visitor := NewWindowVisitor(root)

		for {
			select {
			case command := <-commandChan:
				fmt.Print(command)
			case in := <-userInputChan:
				switch in {
				case byte(ESC):

					//read the rest
					seq := []byte{in, <-userInputChan}
					for b := range userInputChan {
						seq = append(seq, b)
						if b >= 0x40 && b <= 0x7E { //final byte reached
							break
						}
					}
					// interpret
					switch seq[len(seq)-1] {
					case 'R':
						// window resized
						var w, h int
						fmt.Sscanf(string(seq), "\x1b[%d;%dR", &h, &w)
						dimensionsChan <- w
						dimensionsChan <- h
					}
				case KEY_CYCLE:
					visitor.Current().Deselect()
					visitor.Next().Select()
				case KEY_QUIT:
					endTerminalLoop(oldState)
					quitChan <- struct{}{}
					return
				default:
					if !visitor.Current().ResolveInput(in) {
						leftoverInput <- in
					}
				}
			}
		}
	}()

}

func endTerminalLoop(oldState *term.State) {
	//clear screen
	fmt.Printf("%c[H%c[J", ESC, ESC)
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

func GetWindowDimensionsFunc(comChan chan<- Com, dimChan <-chan int) func() (int, int) {
	return func() (w int, h int) {
		comChan <- "\x1b[999;999H\x1b[6n\x1b[0;0H"
		w = <-dimChan
		h = <-dimChan

		return w, h
	}
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
