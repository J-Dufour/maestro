package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/J-Dufour/maestro/audio"
)

type OuterWindowController struct {
	win *Window

	title string
}

func OuterWindowControllerFunc(title string, innerController func(*Window) Controller) func(*Window) Controller {
	return func(w *Window) Controller {
		out := &OuterWindowController{w, title}
		out.Resize()

		inner := w.NewInnerChild(1)
		inner.SetController(innerController)
		return out
	}
}

func (o *OuterWindowController) Resize() {
	w, h := o.win.GetDimensions()

	o.win.GetOffsetComBuilder().DrawBox(Box{0, 0, uint(w), uint(h)}, o.title).Exec()
}

type QueueWindowController struct {
	win *Window

	queue     []audio.Metadata
	sourceIdx int

	maxTitleLen int
	maxIdxLen   int
	maxHeight   int
}

func QueueWindowControllerFunc(player *audio.Player) func(*Window) Controller {
	return OuterWindowControllerFunc(" Queue ",
		func(w *Window) Controller {
			controller := &QueueWindowController{}
			controller.win = w
			controller.queue = make([]audio.Metadata, 0)
			controller.sourceIdx = 0

			controller.Resize()

			controller.startQueueWindowLoop(player)
			return controller
		})
}

func (q *QueueWindowController) Resize() {
	q.UpdateQueue(q.queue)
}

func (q *QueueWindowController) UpdateQueue(queue []audio.Metadata) {
	q.queue = queue

	// grab dimensions
	width, height := q.win.GetDimensions()
	q.maxHeight = height

	if length := len(q.queue); length == 0 {
		q.maxIdxLen = 1
	} else {
		q.maxIdxLen = 1 + int(math.Log10(float64(length)))
	}

	q.maxTitleLen = width - q.maxIdxLen - 2

	// draw queue
	builder := q.win.GetOffsetComBuilder()
	for i, source := range q.queue {
		if i >= q.maxHeight {
			break
		}
		builder.Write(q.getQueueLine(i+1, source)).MoveLines(1)
	}
	builder.Exec()

	// highlight
	q.Highlight(q.sourceIdx)
}

func (q *QueueWindowController) getQueueLine(idx int, metadata audio.Metadata) string {
	line := metadata.Title
	if q.maxTitleLen > len(line)+6 {
		line += " - " + metadata.Artist
	}

	if len(line) > q.maxTitleLen {
		line = line[:q.maxTitleLen-3] + "..."
	}

	return fmt.Sprintf("%*d. %-*s", q.maxIdxLen, idx, q.maxTitleLen, line)
}

func (q *QueueWindowController) Highlight(idx int) {
	if q.sourceIdx >= len(q.queue) {
		return
	}
	//un-highlight previous
	metadata := q.queue[q.sourceIdx]
	builder := q.win.GetOffsetComBuilder()
	builder.MoveLines(q.sourceIdx).SelectGraphicsRendition(POSITIVE).Write(q.getQueueLine(q.sourceIdx+1, metadata))

	q.sourceIdx = idx
	if q.sourceIdx >= len(q.queue) {
		return
	}

	metadata = q.queue[q.sourceIdx]
	builder.MoveTo(1, uint(q.sourceIdx+1)).SelectGraphicsRendition(NEGATIVE).Write(q.getQueueLine(q.sourceIdx+1, metadata)).ClearGraphicsRendition()
	builder.Exec()
}

func (q *QueueWindowController) startQueueWindowLoop(player *audio.Player) {
	queueUpdated := make(chan struct{})
	songUpdated := make(chan struct{})

	player.SubscribeToQueueUpdate(queueUpdated)
	player.SubscribeToSourceChange(songUpdated)

	go func() {
		for {
			select {
			case <-queueUpdated:
				// redraw queue
				q.UpdateQueue(player.GetQueue())
			case <-songUpdated:
				// clear previous
				q.Highlight(player.GetPositionInQueue())
			}
		}
	}()
}

type PlayerWindowController struct {
	win *Window

	metadata audio.Metadata

	infoLines []int
	trackLine int

	w int
	h int
}

const (
	LINE_START = '├'
	LINE_MID   = '─'
	LINE_END   = '┤'

	CURSOR_START = '┠'
	CURSOR_MID   = '┼'
	CURSOR_END   = '┨'
)

func PlayerWindowControllerFunc(player *audio.Player) func(*Window) Controller {
	return OuterWindowControllerFunc(" Player ", func(w *Window) Controller {
		controller := &PlayerWindowController{w, *audio.NewMetadata(), []int{}, 1, 0, 0}
		controller.Resize()

		controller.startPlayerWindowLoop(player)

		return controller
	})

}

func (p *PlayerWindowController) Resize() {
	p.w, p.h = p.win.GetDimensions()

	// center info
	switch {
	case p.h == 1:
		p.infoLines = []int{}
		p.trackLine = 1
	case p.h == 2:
		p.infoLines = []int{1}
		p.trackLine = 2
	case p.h == 3:
		p.infoLines = []int{1, 2}
		p.trackLine = 3
	case p.h <= 7:
		start := (p.h-4)/2 + 1
		p.infoLines = []int{start, start + 1, start + 2}
		p.trackLine = start + 3
	default:
		start := (p.h-7)/2 + 1
		p.infoLines = []int{start, start + 2, start + 4}
		p.trackLine = start + 6
	}

	p.SetTrackPosition(0)
	p.SetNewMetadata(p.metadata)
}

func (p *PlayerWindowController) SetNewMetadata(source audio.Metadata) {
	p.metadata = source

	//draw
	switch len(p.infoLines) {
	case 1:
		line := centeredString(concatMax(p.w, " - ", p.metadata.Title, p.metadata.Album, p.metadata.Artist), p.w)
		p.win.GetOffsetComBuilder().MoveTo(1, uint(p.infoLines[0])).Write(line).Exec()
	case 2:
		line1 := centeredString(concatMax(p.w, " - ", p.metadata.Title, p.metadata.Album), p.w)
		line2 := centeredString(concatMax(p.w, " - ", p.metadata.Artist), p.w)

		p.win.GetOffsetComBuilder().MoveTo(1, uint(p.infoLines[0])).Write(line1).MoveTo(1, uint(p.infoLines[1])).Write(line2).Exec()
	case 3:
		line1 := centeredString(concatMax(p.w, "", p.metadata.Title), p.w)
		line2 := centeredString(concatMax(p.w, "", p.metadata.Album), p.w)
		line3 := centeredString(concatMax(p.w, "", p.metadata.Artist), p.w)
		p.win.GetOffsetComBuilder().MoveTo(1, uint(p.infoLines[0])).Write(line1).MoveTo(1, uint(p.infoLines[1])).Write(line2).MoveTo(1, uint(p.infoLines[2])).Write(line3).Exec()
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

func (p *PlayerWindowController) SetTrackPosition(pos int64) {
	duration := p.metadata.Duration
	var realPos int
	if duration == 0 {
		realPos = 0
	} else {
		ratio := float64(pos) / float64(duration)
		realPos = int(ratio * float64(p.w))
		if realPos > p.w-1 {
			realPos = p.w - 1
		}
	}
	var cursor rune
	switch realPos {
	case 0:
		cursor = CURSOR_START
	case p.w - 1:
		cursor = CURSOR_END
	default:
		cursor = CURSOR_MID
	}

	p.win.GetOffsetComBuilder().MoveTo(1, uint(p.trackLine)).Write(LINE_START, strings.Repeat(string(LINE_MID), p.w-2), LINE_END).MoveTo(uint(realPos)+1, uint(p.trackLine)).Write(cursor).Exec()
}

func (p *PlayerWindowController) startPlayerWindowLoop(player *audio.Player) {
	songUpdated := make(chan struct{})
	player.SubscribeToSourceChange(songUpdated)

	go func() {
		period := 100 * time.Millisecond
		clock := time.NewTicker(period)
		for {
			select {
			case <-songUpdated:
				if idx := player.GetPositionInQueue(); idx < len(player.GetQueue()) {
					p.SetNewMetadata(player.GetQueue()[idx])
				}
				p.SetTrackPosition(int64(player.GetPositionInTrack()))
			case <-clock.C:
				p.SetTrackPosition(int64(player.GetPositionInTrack()))
			}
		}
	}()
}
