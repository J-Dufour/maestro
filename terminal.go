package main

import (
	"fmt"
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

type Com string
type ComBuilder struct {
	canvas  Box
	builder *strings.Builder
	channel chan Com
}

func NewCom() *ComBuilder { return &ComBuilder{} }

func (cb *ComBuilder) MoveTo(x uint, y uint) *ComBuilder {
	cb.builder.WriteString(fmt.Sprintf("%c[%d;%dH", ESC, cb.canvas.y+y, cb.canvas.x+x))
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
		cb.Offset(int(cb.canvas.x), 0)
	} else if lines < 0 {
		cb.builder.WriteString(fmt.Sprintf("%c[%dF", ESC, -lines))
		cb.Offset(int(cb.canvas.x), 0)
	}
	return cb
}

func (cb *ComBuilder) Clear() *ComBuilder {
	clearString := strings.Repeat(" ", int(cb.canvas.w))
	cb.MoveTo(1, 1)
	for i := 0; i < int(cb.canvas.h); i++ {
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

func (cb *ComBuilder) PermaOffset(x, y uint) *ComBuilder {
	cb.canvas.x += x
	cb.canvas.y += y
	return cb.Offset(int(x), int(y))
}

func (cb *ComBuilder) ChangeDimensions(w, h uint) *ComBuilder {
	cb.canvas.w, cb.canvas.h = w, h
	return cb
}

func (cb *ComBuilder) Exec() {
	// sends command to window
	cb.channel <- cb.BuildCom()
}

type parentWindowCreator func(*BaseWindow) ParentWindow

type ParentWindow interface {
	Window

	NewChild() Window
	ReplaceChild(Window, Window) bool
	AddChild(Window)
	SwapPos(int, int)
}

type Window interface {
	GetParent() ParentWindow

	Encapsulate(parentWindowCreator)

	GetChildren() []Window

	GetDimensions() (int, int)
	getBox() Box
	Resize(b Box)

	GetOffsetComBuilder() *ComBuilder
	Exec(Com)

	SetController(Controller)
	ResolveInput(b byte) bool
	Select()
	Deselect()
	isSelectable() bool
	SetSelectable(bool)
}

func GetRoot(w Window) Window {
	if w.GetParent() == nil {
		return w
	} else {
		return GetRoot(w.GetParent())
	}
}

type stackWindow1D struct {
	BaseWindow

	children         []Window
	childProportions []float64
}

func (win *stackWindow1D) SwapPos(a, b int) {
	if 0 <= a && a < len(win.children) && 0 <= b && b < len(win.children) {
		win.children[a], win.children[b] = win.children[b], win.children[a]
	}
}

func (win *stackWindow1D) GetChildren() []Window {
	return win.children
}

func split(width int, proportions []float64) []int {
	if len(proportions) == 0 {
		return []int{}
	}

	total := float64(0)
	for _, val := range proportions {
		total += val
	}

	ratio := proportions[0] / total

	out := int(float64(width) * ratio)
	if out > width {
		out = width
	}

	return append([]int{out}, split(width-out, proportions[1:])...)
}

type VerticalStackWindow struct{ stackWindow1D }

func NewVerticalStackWindow(b *BaseWindow) ParentWindow {
	return &VerticalStackWindow{stackWindow1D{*b, []Window{}, []float64{}}}
}

func (win *VerticalStackWindow) NewChild() (child Window) {
	child = &BaseWindow{win, win.Box, win.coms, nil, true, false}
	win.AddChild(child)

	return child
}

func (win *VerticalStackWindow) AddChild(child Window) {
	win.childProportions = append(win.childProportions, 1)
	win.children = append(win.children, child)
	win.Resize(win.Box)
}

func (win *VerticalStackWindow) ReplaceChild(old Window, new Window) bool {
	for i, v := range win.children {
		if v == old {
			win.children[i] = new
			win.Resize(win.Box)
			return true
		}
	}
	return false
}

func (win *VerticalStackWindow) Resize(b Box) {
	win.Box = b

	if win.con != nil {
		win.con.Resize(int(win.w), int(win.h))
	}

	newHeights := split(int(win.h), win.childProportions)
	pos := 0
	for i, w := range win.children {
		newY := uint(pos)
		newH := uint(newHeights[i])

		pos += newHeights[i]
		w.Resize(Box{0, newY, win.w, newH})
	}
}

type HorizontalStackWindow struct{ stackWindow1D }

func NewHorizontalStackWindow(b *BaseWindow) ParentWindow {
	return &HorizontalStackWindow{stackWindow1D{*b, []Window{}, []float64{}}}
}

func (win *HorizontalStackWindow) NewChild() (child Window) {
	child = &BaseWindow{win, win.Box, win.coms, nil, true, false}
	win.AddChild(child)

	return child
}

func (win *HorizontalStackWindow) AddChild(child Window) {
	win.childProportions = append(win.childProportions, 1)
	win.children = append(win.children, child)
	win.Resize(win.Box)
}

func (win *HorizontalStackWindow) ReplaceChild(old Window, new Window) bool {
	for i, v := range win.children {
		if v == old {
			win.children[i] = new
			win.Resize(win.Box)
			return true
		}
	}
	return false
}

func (win *HorizontalStackWindow) Resize(b Box) {
	win.Box = b

	if win.con != nil {
		win.con.Resize(int(win.w), int(win.h))
	}

	newWidths := split(int(win.w), win.childProportions)
	pos := 0
	for i, w := range win.children {
		newX := uint(pos)
		newW := uint(newWidths[i])

		pos += newWidths[i]
		w.Resize(Box{newX, 0, newW, win.h})
	}
}

type ContainerWindow struct {
	BaseWindow

	child Window
}

func NewContainerWindow(b *BaseWindow) ParentWindow {
	return &ContainerWindow{*b, nil}
}

func (win *ContainerWindow) NewChild() (child Window) {
	if win.child != nil {
		return nil
	}

	childBox := Box{1, 1, win.w - 2, win.h - 2}
	win.AddChild(&BaseWindow{win, childBox, win.coms, win.con, true, false})
	return win.child
}

func (win *ContainerWindow) AddChild(child Window) {
	if win.child != nil {
		return
	}

	win.child = child
	win.Resize(win.Box)
}

func (win *ContainerWindow) ReplaceChild(old, new Window) bool {
	if win.child == old {
		win.child = new
		return true
	}
	return false
}

func (win *ContainerWindow) SwapPos(a, b int) {}

func (win *ContainerWindow) Resize(b Box) {
	win.Box = b

	if win.con != nil {
		win.con.Resize(int(win.w), int(win.h))
	}

	if win.child != nil {
		win.child.Resize(Box{1, 1, win.w - 2, win.h - 2})
	}
}

func (win *ContainerWindow) GetChildren() []Window {
	if win.child != nil {
		return []Window{win.child}
	} else {
		return []Window{}
	}
}

type BaseWindow struct {
	parent ParentWindow
	Box

	coms chan Com
	con  Controller

	selectable bool
	selected   bool
}

func (win *BaseWindow) GetDimensions() (w int, h int) {
	return int(win.w), int(win.h)
}

func (win *BaseWindow) WithinBounds(box Box) bool {
	//check if top left corner is within bounds
	inTopL := box.x < win.w && box.y < win.h
	//check if bottom right corner is within bounds
	inBotR := box.x+box.w <= win.w && box.y+box.h <= win.h

	return inTopL && inBotR

}

func (win *BaseWindow) GetOffsetComBuilder() *ComBuilder {
	var cb *ComBuilder
	if win.parent != nil {
		cb = win.parent.GetOffsetComBuilder()

		//Offset
		cb.PermaOffset(win.x, win.y)
		cb.ChangeDimensions(win.w, win.h)
	} else {
		cb = &ComBuilder{Box{0, 0, win.w, win.h}, &strings.Builder{}, win.coms}
		cb.MoveTo(0, 0)
	}

	return cb
}

func (win *BaseWindow) Resize(b Box) {
	win.Box = b

	if win.con != nil {
		win.con.Resize(int(win.w), int(win.h))
	}
}

func (win *BaseWindow) Encapsulate(parentCreator parentWindowCreator) {

	newParent := parentCreator(&BaseWindow{win.parent, win.Box, win.coms, nil, false, false})
	if win.parent != nil {
		win.parent.ReplaceChild(win, newParent)
	}
	newParent.AddChild(win)
	win.parent = newParent
}

func (win *BaseWindow) Exec(com Com) {
	win.coms <- com
}

func (win *BaseWindow) SetController(c Controller) {
	// terminate old controller
	if win.con != nil {
		win.con.Terminate()
	}

	// initiate new controller
	win.con = c
	win.con.Init(win.GetOffsetComBuilder, area{int(win.w), int(win.h)}, win.selected)
}

func (win *BaseWindow) Select() {
	win.selected = true
	if win.con != nil {
		win.con.Select()
	}
	if win.parent != nil {
		win.parent.Select()
	}
}

func (win *BaseWindow) Deselect() {
	win.selected = false
	if win.con != nil {
		win.con.Deselect()
	}
	if win.parent != nil {
		win.parent.Deselect()
	}
}

func (win *BaseWindow) isSelectable() bool {
	return win.selectable
}

func (win *BaseWindow) ResolveInput(b byte) bool {
	if win.con == nil || !win.con.ResolveInput(b) {
		if win.parent != nil {
			return win.parent.ResolveInput(b)
		} else {
			return false
		}
	}
	return true
}

func (win *BaseWindow) getBox() Box {
	return win.Box
}

func (win *BaseWindow) SetSelectable(b bool) {
	win.selectable = b
}

func (win *BaseWindow) GetParent() ParentWindow {
	return win.parent
}

func (win *BaseWindow) GetChildren() []Window {
	return []Window{}
}

func HSplit(w Window) Window {
	w.Encapsulate(NewHorizontalStackWindow)
	return addSibling(w)
}

func VSplit(w Window) Window {
	w.Encapsulate(NewVerticalStackWindow)
	return addSibling(w)
}

func addSibling(child Window) Window {
	if child.GetParent() == nil {
		return nil
	}

	out := child.GetParent().NewChild()
	root := GetRoot(child)
	w, h := root.GetDimensions()
	rootBox := Box{1, 1, uint(w), uint(h)}
	root.GetOffsetComBuilder().Clear().Exec()
	root.Resize(rootBox)

	return out
}

type Controller interface {
	Init(builderFactory func() *ComBuilder, dimensions area, selected bool)

	Select()
	Deselect()
	Resize(int, int)

	ResolveInput(byte) bool
	Terminate()
}

type WindowVisitor struct {
	cur     Window
	history []int
}

func NewWindowVisitor(win Window) *WindowVisitor {
	return &WindowVisitor{win, []int{-1}}
}

func (v *WindowVisitor) Current() Window {
	return v.cur
}

func (v *WindowVisitor) Next() Window {
	// increment latest idx
	last := len(v.history) - 1
	v.history[last]++

	// check if valid child exists
	if len(v.cur.GetChildren()) > v.history[last] {
		// get child
		v.cur = v.cur.GetChildren()[v.history[last]]
	} else {
		// go up one layer in history
		v.history = v.history[:last]
		if len(v.history) == 0 { // root changed since initialization. add layer to history
			v.history = []int{0}
		}

		// move to parent if exists
		if v.cur.GetParent() != nil {
			v.cur = v.cur.GetParent()
			return v.Next()
		}
	}
	v.history = append(v.history, -1)
	if v.cur.isSelectable() {
		return v.cur
	} else {
		return v.Next()
	}
}

func InitTerminalLoop() (root Window, quit chan struct{}, globalInput chan byte) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Println(err)
		return
	}

	//define root window
	quitChan := make(chan struct{})
	doneChan := make(chan struct{})
	commands := make(chan Com)
	go terminalLoop(oldState, commands, quitChan, doneChan)

	root = &BaseWindow{nil, Box{1, 1, 0, 0}, commands, nil, true, false}
	dimensions := make(chan int)
	leftover := make(chan byte, 8)
	go inputLoop(root, leftover, dimensions, quitChan)

	GetDimensions := GetWindowDimensionsFunc(commands, dimensions)
	w, h := GetDimensions()
	root.Resize(Box{1, 1, uint(w), uint(h)})

	// resize loop
	go func() {
		curW, curH := GetDimensions()
		resizeClock := time.NewTicker(100 * time.Millisecond)

		for {
			<-resizeClock.C
			w, h := GetDimensions()

			if w != curW || h != curH {
				commands <- "\x1b[2J\x1b[3J\x1b[H"
				curW, curH = w, h
				GetRoot(root).Resize(Box{1, 1, uint(curW), uint(curH)})
			}
		}
	}()
	root.GetOffsetComBuilder().Clear().Exec()
	return root, doneChan, leftover
}

func terminalLoop(oldState *term.State, commandChan <-chan Com, quitChan <-chan struct{}, doneChan chan<- struct{}) {
	defer endTerminalLoop(oldState)

	// hide cursor
	fmt.Printf("%c[?25l", ESC)

	for {
		select {
		case command := <-commandChan:
			fmt.Print(command)
		case <-quitChan:
			endTerminalLoop(oldState)
			doneChan <- struct{}{}
			return

		}
	}
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

func inputLoop(root Window, input chan byte, dimensionsChan chan int, quitChan chan struct{}) {
	visitor := NewWindowVisitor(root)

	char := make([]byte, 1)
	for {
		count, _ := os.Stdin.Read(char)
		if count > 0 {
			in := char[0]
			switch in {
			case byte(ESC):

				//read the rest
				os.Stdin.Read(char)
				seq := []byte{in, char[0]}
				for {
					os.Stdin.Read(char)
					b := char[0]
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
				quitChan <- struct{}{}
				return
			case KEY_SPLIT:
				HSplit(visitor.Current())
			default:
				if !visitor.Current().ResolveInput(in) {
					input <- in
				}
			}

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
