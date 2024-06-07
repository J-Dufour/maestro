package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/J-Dufour/maestro/audio"
)

type QueueWindowController struct {
	win *Window

	queue     []audio.AudioSource
	sourceIdx int

	maxTitleLen int
	maxIdxLen   int
	maxHeight   int
}

func NewQueueWindowController(w *Window) *QueueWindowController {
	controller := &QueueWindowController{}
	controller.win = w
	controller.queue = make([]audio.AudioSource, 0)
	controller.sourceIdx = 0

	controller.Resize()

	return controller
}

func (q *QueueWindowController) Resize() {
	width, height := q.win.GetDimensions()
	q.maxHeight = height

	if length := len(q.queue); length == 0 {
		q.maxIdxLen = 1
	} else {
		q.maxIdxLen = 1 + int(math.Log10(float64(length)))
	}

	q.maxTitleLen = width - q.maxIdxLen - 2
}

func (q *QueueWindowController) UpdateQueue(queue []audio.AudioSource) {
	q.queue = queue
	q.Resize()

	// draw queue
	builder := q.win.GetOffsetComBuilder()
	for i, source := range q.queue {
		if i >= q.maxHeight {
			break
		}
		builder.Write(q.getQueueLine(i+1, source.GetMetadata())).MoveLines(1)
	}
	q.win.Exec(builder.BuildCom())

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
	//un-highlight previous
	metadata := q.queue[q.sourceIdx].GetMetadata()
	builder := q.win.GetOffsetComBuilder()
	builder.MoveLines(q.sourceIdx).SelectGraphicsRendition(POSITIVE).Write(q.getQueueLine(q.sourceIdx+1, metadata))

	q.sourceIdx = idx
	if q.sourceIdx >= len(q.queue) {
		return
	}

	metadata = q.queue[q.sourceIdx].GetMetadata()
	builder.MoveTo(1, uint(q.sourceIdx+1)).SelectGraphicsRendition(NEGATIVE).Write(q.getQueueLine(q.sourceIdx+1, metadata)).ClearGraphicsRendition()
	q.win.Exec(builder.BuildCom())
}

func StartQueueWindowLoop(con *QueueWindowController, player *audio.Player) {
	queueUpdated := make(chan struct{})
	songUpdated := make(chan struct{})

	player.SubscribeToQueueUpdate(queueUpdated)
	player.SubscribeToSourceChange(songUpdated)

	go func() {
		for {
			select {
			case <-queueUpdated:
				// redraw queue
				con.UpdateQueue(player.GetQueue())
			case <-songUpdated:
				// clear previous
				con.Highlight(player.GetPositionInQueue())
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

func NewPlayerWindowController(win *Window) *PlayerWindowController {
	controller := &PlayerWindowController{win, *audio.NewMetadata(), []int{}, 1, 0, 0}
	controller.Resize()

	return controller
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
	case p.h < 7:
		start := (p.h-4)/2 + 1
		p.infoLines = []int{start, start + 1, start + 2}
		p.trackLine = start + 3
	default:
		start := (p.h-7)/2 + 1
		p.infoLines = []int{start, start + 2, start + 4}
		p.trackLine = start + 6
	}
}

func (p *PlayerWindowController) SetNewSource(source audio.AudioSource) {
	p.metadata = source.GetMetadata()

	//draw
	switch len(p.infoLines) {
	case 1:
		line := centeredString(concatMax(p.w, " - ", p.metadata.Title, p.metadata.Album, p.metadata.Artist), p.w)
		p.win.Exec(p.win.GetOffsetComBuilder().MoveTo(1, uint(p.infoLines[0])).Write(line).BuildCom())
	case 2:
		line1 := centeredString(concatMax(p.w, " - ", p.metadata.Title, p.metadata.Album), p.w)
		line2 := centeredString(concatMax(p.w, " - ", p.metadata.Artist), p.w)

		p.win.Exec(p.win.GetOffsetComBuilder().MoveTo(1, uint(p.infoLines[0])).Write(line1).MoveTo(1, uint(p.infoLines[1])).Write(line2).BuildCom())
	case 3:
		line1 := centeredString(concatMax(p.w, "", p.metadata.Title), p.w)
		line2 := centeredString(concatMax(p.w, "", p.metadata.Album), p.w)
		line3 := centeredString(concatMax(p.w, "", p.metadata.Artist), p.w)
		p.win.Exec(p.win.GetOffsetComBuilder().MoveTo(1, uint(p.infoLines[0])).Write(line1).MoveTo(1, uint(p.infoLines[1])).Write(line2).MoveTo(1, uint(p.infoLines[2])).Write(line3).BuildCom())
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
	ratio := float64(pos) / float64(duration)
	realPos := int(ratio * float64(p.w))
	if realPos > p.w-1 {
		realPos = p.w - 1
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

	p.win.Exec(p.win.GetOffsetComBuilder().MoveTo(1, uint(p.trackLine)).Write(LINE_START, strings.Repeat(string(LINE_MID), p.w-2), LINE_END).MoveTo(uint(realPos)+1, uint(p.trackLine)).Write(cursor).BuildCom())
}

func StartPlayerWindowLoop(p *PlayerWindowController, player *audio.Player) {
	songUpdated := make(chan struct{})
	player.SubscribeToSourceChange(songUpdated)

	go func() {
		period := 500 * time.Millisecond
		clock := time.NewTicker(period)
		for {
			select {
			case <-songUpdated:
				p.SetNewSource(player.GetQueue()[player.GetPositionInQueue()])
				p.SetTrackPosition(int64(player.GetPositionInTrack()))
			case <-clock.C:
				p.SetTrackPosition(int64(player.GetPositionInTrack()))
			}
		}
	}()
}
