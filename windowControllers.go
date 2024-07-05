package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/J-Dufour/maestro/audio"
)

type area struct {
	w, h int
}

type ControllerChannels struct {
	InputChan     chan byte
	ResizeChan    chan area
	TerminateChan chan struct{}
	SelectChan    chan bool
}

func NewControllerChannels() ControllerChannels {
	return ControllerChannels{
		InputChan:     make(chan byte, 16),
		ResizeChan:    make(chan area),
		TerminateChan: make(chan struct{}),
		SelectChan:    make(chan bool),
	}
}

type BaseWindowController struct {
	inputRange string

	ControllerChannels

	loop func(func() *ComBuilder, ControllerChannels)
}

func NewBaseWindowController(loop func(func() *ComBuilder, ControllerChannels), inputRange string) *BaseWindowController {
	out := &BaseWindowController{}

	out.ControllerChannels = NewControllerChannels()

	out.loop = loop

	return out
}

func (c *BaseWindowController) Init(builderFactory func() *ComBuilder, dimensions area, selected bool) {
	go c.loop(builderFactory, c.ControllerChannels)
	c.ResizeChan <- dimensions
	if selected {
		c.SelectChan <- true
	}
}

func (c *BaseWindowController) Select()   { c.SelectChan <- true }
func (c *BaseWindowController) Deselect() { c.SelectChan <- false }
func (c *BaseWindowController) ResolveInput(b byte) bool {
	if strings.ContainsRune(c.inputRange, rune(b)) {
		select {
		case c.InputChan <- b:
		default:
		}
		return true
	} else {
		return false
	}
}
func (c *BaseWindowController) Resize(w, h int) {
	c.ResizeChan <- area{w, h}
}
func (c *BaseWindowController) Terminate() {
	c.TerminateChan <- struct{}{}
}

type BorderedWindowController struct {
	newCom   func() *ComBuilder
	title    string
	w, h     int
	selected bool

	inner Controller
}

func NewBorderedWindowController(title string, inner Controller) Controller {
	return &BorderedWindowController{nil, title, 0, 0, false, inner}
}

func (b *BorderedWindowController) Init(builderFactory func() *ComBuilder, dimensions area, selected bool) {
	b.newCom = builderFactory

	b.w, b.h = dimensions.w, dimensions.h

	b.selected = selected

	b.Draw()
	if b.inner != nil {
		innerFactory := func() *ComBuilder {
			return builderFactory().PermaOffset(1, 1).ChangeDimensions(uint(b.w-2), uint(b.h-2))
		}

		b.inner.Init(innerFactory, area{b.w - 2, b.h - 2}, selected)
	}
}

func (b *BorderedWindowController) Select() {
	b.selected = true
	b.Draw()

	if b.inner != nil {
		b.inner.Select()
	}
}

func (b *BorderedWindowController) Deselect() {
	b.selected = false
	b.Draw()

	if b.inner != nil {
		b.inner.Deselect()
	}
}

func (b *BorderedWindowController) Resize(w, h int) {
	b.w, b.h = w, h
	b.Draw()

	if b.inner != nil {
		if w < 2 {
			w = 2
		}
		if h < 2 {
			h = 2
		}
		b.inner.Resize(w-2, h-2)
	}
}

func (b *BorderedWindowController) ResolveInput(by byte) bool {
	if b.inner != nil {
		return b.inner.ResolveInput(by)
	} else {
		return false
	}
}

func (b *BorderedWindowController) Terminate() {
	if b.inner != nil {
		b.inner.Terminate()
	}
}

func (b *BorderedWindowController) Draw() {
	b.newCom().DrawBox(Box{0, 0, uint(b.w), uint(b.h)}, b.title, b.selected).Exec()
}

func NewQueueWindowController(player *audio.Player) Controller {
	return NewBaseWindowController(func(buildCom func() *ComBuilder, con ControllerChannels) {
		queueUpdated := make(chan struct{})
		songUpdated := make(chan struct{})

		player.SubscribeToQueueUpdate(queueUpdated)
		player.SubscribeToSourceChange(songUpdated)

		queue := player.GetQueue()
		curIdx := 0
		maxIdxLen := 1

		dims := area{0, 0}
		for {
			select {
			case <-queueUpdated:
				queue = player.GetQueue()
				if length := len(queue); length == 0 {
					maxIdxLen = 1
				} else {
					maxIdxLen = 1 + int(math.Log10(float64(length)))
				}
				DrawQueue(buildCom(), queue, curIdx, maxIdxLen, dims.w, dims.h)

			case <-songUpdated:
				builder := buildCom()
				if curIdx < len(queue) {
					builder.MoveTo(1, uint(curIdx+1))
					WriteQueueLine(builder, curIdx+1, maxIdxLen, queue[curIdx], dims.w-2-maxIdxLen, false)
				}
				curIdx = player.GetPositionInQueue()
				if curIdx < len(queue) {
					builder.MoveTo(1, uint(curIdx+1))
					WriteQueueLine(builder, curIdx+1, maxIdxLen, queue[curIdx], dims.w-2-maxIdxLen, true)
				}
				builder.Exec()

			case newDims := <-con.ResizeChan:
				dims = newDims
				DrawQueue(buildCom(), queue, curIdx, maxIdxLen, dims.w, dims.h)
			case <-con.TerminateChan:
				return
			case <-con.SelectChan:
			case <-con.InputChan:
			}
		}
	}, "")
}

func DrawQueue(builder *ComBuilder, queue []audio.Metadata, curIdx, maxIdxLen, w, h int) {
	maxTitleLen := w - maxIdxLen - 2

	// draw queue
	for i, source := range queue {
		if i >= h {
			break
		}

		WriteQueueLine(builder, i+1, maxIdxLen, source, maxTitleLen, i == curIdx).MoveLines(1)
	}
	builder.Exec()
}

func WriteQueueLine(builder *ComBuilder, idx int, maxIdx int, metadata audio.Metadata, maxW int, highlighted bool) *ComBuilder {
	graphics := POSITIVE
	if highlighted {
		graphics = NEGATIVE
	}

	line := metadata.Title
	if maxW > len(line)+6 {
		line += " - " + metadata.Artist
	}

	if len(line) > maxW {
		line = line[:maxW-3] + "..."
	}

	return builder.SelectGraphicsRendition(graphics).Write(fmt.Sprintf("%*d. %-*s", maxIdx, idx, maxW, line)).ClearGraphicsRendition()
}

const (
	LINE_START = '├'
	LINE_MID   = '─'
	LINE_END   = '┤'

	CURSOR_START = '┠'
	CURSOR_MID   = '┼'
	CURSOR_END   = '┨'
)

func NewPlayerWindowController(player *audio.Player) Controller {
	return NewBaseWindowController(func(buildCom func() *ComBuilder, con ControllerChannels) {
		songUpdated := make(chan struct{})
		player.SubscribeToSourceChange(songUpdated)

		curSource := *audio.NewMetadata()

		dims := area{0, 0}
		infoLines := []int{1}

		period := 100 * time.Millisecond
		clock := time.NewTicker(period)

		for {
			select {
			case <-songUpdated:
				curSource = player.GetQueue()[player.GetPositionInQueue()]
				DrawInfo(buildCom(), curSource, infoLines, int64(player.GetPositionInTrack()), dims)

			case newDims := <-con.ResizeChan:
				dims = newDims
				infoLines = infoLinesFromHeight(dims.h)
				DrawInfo(buildCom(), curSource, infoLines, int64(player.GetPositionInTrack()), dims)
			case <-clock.C:
				DrawTrack(buildCom(), curSource, int64(player.GetPositionInTrack()), infoLines[len(infoLines)-1], dims)
			case <-con.TerminateChan:
				return
			case <-con.SelectChan:
			case <-con.InputChan:
			}
		}
	}, "")
}

func infoLinesFromHeight(h int) []int {
	switch {
	case h == 1:
		return []int{1}
	case h == 2:
		return []int{1, 2}
	case h == 3:
		return []int{1, 2, 3}
	case h <= 7:
		start := (h-4)/2 + 1
		return []int{start, start + 1, start + 2, start + 3}
	default:
		start := (h-7)/2 + 1
		return []int{start, start + 2, start + 4, start + 6}
	}
}

func DrawInfo(builder *ComBuilder, source audio.Metadata, lines []int, pos int64, dims area) {
	lastIdx := len(lines) - 1
	DrawMetadata(builder, source, lines[:lastIdx], dims)
	DrawTrack(builder, source, pos, lines[lastIdx], dims)
}
func DrawMetadata(builder *ComBuilder, source audio.Metadata, lines []int, dims area) {
	//draw
	switch len(lines) {
	case 0:

	case 1:
		line := centeredString(concatMax(dims.w, " - ", source.Title, source.Album, source.Artist), dims.w)
		builder.MoveTo(1, uint(lines[0])).Write(line).Exec()
	case 2:
		line1 := centeredString(concatMax(dims.w, " - ", source.Title, source.Album), dims.w)
		line2 := centeredString(concatMax(dims.w, " - ", source.Artist), dims.w)

		builder.MoveTo(1, uint(lines[0])).Write(line1).MoveTo(1, uint(lines[1])).Write(line2).Exec()
	case 3:
		line1 := centeredString(concatMax(dims.w, "", source.Title), dims.w)
		line2 := centeredString(concatMax(dims.w, "", source.Album), dims.w)
		line3 := centeredString(concatMax(dims.w, "", source.Artist), dims.w)
		builder.MoveTo(1, uint(lines[0])).Write(line1).MoveTo(1, uint(lines[1])).Write(line2).MoveTo(1, uint(lines[2])).Write(line3).Exec()
	}
}

func concatMax(max int, separator string, strings ...string) (concat string) {
	concat = strings[0]
	for _, str := range strings[1:] {
		if len(concat)+len(separator)+3 >= max {
			break
		}
		concat += separator + str
	}

	if len(concat) > max {
		concat = concat[:max-3] + "..."
	}
	return concat
}

func centeredString(str string, width int) string {
	offset := (width - len(str)) / 2
	out := strings.Repeat(" ", offset) + str
	out += strings.Repeat(" ", width-len(out))
	return out

}

func DrawTrack(builder *ComBuilder, source audio.Metadata, pos int64, trackHeight int, dims area) {
	duration := source.Duration
	var realPos int
	if duration == 0 {
		realPos = 0
	} else {
		ratio := float64(pos) / float64(duration)
		realPos = int(ratio * float64(dims.w))
		if realPos > dims.w-1 {
			realPos = dims.w - 1
		}
	}
	var cursor rune
	switch realPos {
	case 0:
		cursor = CURSOR_START
	case dims.w - 1:
		cursor = CURSOR_END
	default:
		cursor = CURSOR_MID
	}

	builder.MoveTo(1, uint(trackHeight)).Write(LINE_START, strings.Repeat(string(LINE_MID), dims.w-2), LINE_END).MoveTo(uint(realPos)+1, uint(trackHeight)).Write(cursor).Exec()
}

// TODO: add new window to select from other windows
type SelectorWindowController struct {
	options []struct {
		title   string
		factory func() Controller
	}
	idx int

	setController func(Controller)

	w, h int

	newCom func() *ComBuilder
}

const (
	KEY_DOWN   = 'j'
	KEY_UP     = 'k'
	KEY_SELECT = '\x0D'
)

func NewSelectorWindowController(possibleControllers []struct {
	title   string
	factory func() Controller
}, setController func(Controller)) Controller {
	return &SelectorWindowController{
		options:       possibleControllers,
		idx:           0,
		setController: setController,
		w:             0,
		h:             0,
		newCom:        nil,
	}
}

func (s *SelectorWindowController) Init(builderFactory func() *ComBuilder, dimensions area, selected bool) {
	s.newCom = builderFactory
	s.w, s.h = dimensions.w, dimensions.h
	s.draw()
}

func (s *SelectorWindowController) Resize(w, h int) {
	s.w, s.h = w, h
	s.draw()
}

func (s *SelectorWindowController) ResolveInput(b byte) bool {
	switch b {
	case KEY_DOWN:
		if s.idx < len(s.options)-1 {
			s.idx++
		}
		s.draw()
	case KEY_UP:
		if s.idx > 0 {
			s.idx--
		}
		s.draw()
	case KEY_SELECT:
		s.setController(s.options[s.idx].factory())
	default:
		return false
	}
	return true
}

func (s *SelectorWindowController) draw() {

	builder := s.newCom()
	list := make([]string, 0)
	idx := s.idx
	if len(s.options) > s.h {
		if idx > s.h {
			shift := idx - s.h + 1
			for _, v := range s.options[shift : shift+s.h] {
				list = append(list, v.title)
			}
			idx = s.h
		}

	} else {
		for _, v := range s.options {
			list = append(list, v.title)
		}
	}

	for i, title := range list {
		if i == idx {
			builder.SelectGraphicsRendition(NEGATIVE).Write(title).SelectGraphicsRendition(POSITIVE).MoveLines(1)
		} else {
			builder.Write(title).MoveLines(1)
		}
	}

	builder.Exec()
}

func (s *SelectorWindowController) Select()    {}
func (s *SelectorWindowController) Deselect()  {}
func (s *SelectorWindowController) Terminate() {}

func min(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}
