package main

import (
	"fmt"
	"math"

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
