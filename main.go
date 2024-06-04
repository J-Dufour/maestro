package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"

	"github.com/J-Dufour/maestro/audio"
)

var VALID_EXT = []string{".mp3", ".wav"}

const (
	KEY_QUIT = 'q'
	KEY_SKIP = 'k'
)

func main() {

	// startup
	audio.InitializeAudioAPI()

	// get file names
	if len(os.Args) < 2 {
		fmt.Println("please provide path(s) to valid music file")
		return
	}

	absolutePaths := make([]string, 0)
	for _, arg := range os.Args[1:] {

		// get absolute paths
		path, err := filepath.Abs(arg)
		if err != nil {
			fmt.Println(err)
			return
		}

		// if folder, find all files within
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			path += "/*"
			subfiles, err := filepath.Glob(path)
			if err == nil {
				for _, subfile := range subfiles {
					if slices.Contains[[]string](VALID_EXT, filepath.Ext(subfile)) {
						if info, err := os.Stat(subfile); err == nil && !info.IsDir() {
							absolutePaths = append(absolutePaths, subfile)
						}
					}
				}
			}
		} else if err == nil && !info.IsDir() {
			// filter by extension
			if slices.Contains[[]string](VALID_EXT, filepath.Ext(path)) {
				absolutePaths = append(absolutePaths, path)
			}
		}

	}

	// get file reader
	sourceProvider := audio.GetAudioSourceProvider()

	sources := make([]audio.AudioSource, 0)
	for _, fileName := range absolutePaths {
		source, err := sourceProvider.GetAudioSourceFromFile(fileName)
		if err != nil {
			fmt.Println(err)
			return
		}

		sources = append(sources, source)
	}

	player, err := audio.NewPlayer()
	if err != nil {
		fmt.Println(err)
		return
	}

	// start UI
	root, initLoop, input := InitTerminalLoop()

	// start input interpreter
	go inputDecoder(input, player)

	// make static queue view
	queue := make([]string, 0)
	for _, source := range sources {
		queue = append(queue, fmt.Sprintf("%s - %s", source.GetMetadata().Title, source.GetMetadata().Artist))
	}
	maxLength := len(queue[0])
	for _, arg := range queue[1:] {
		if maxLength < len(arg) {
			maxLength = len(arg)
		}
	}

	outerQueueWin := root.NewChild(Box{0, 0, 80, 24})
	outerQueueWin.Exec(outerQueueWin.DrawBox(Box{0, 0, 40, 14}, " Queue "))
	innerQueueWin := outerQueueWin.NewChild(Box{1, 1, 38, 12})

	controller := NewQueueWindowController(innerQueueWin)

	StartQueueWindowLoop(controller, player)

	player.AddSourcesToQueue(sources...)
	player.Start()
	initLoop()
}

func inputDecoder(input chan byte, player *audio.Player) {
	for key := range input {
		switch key {
		case KEY_SKIP:
			player.Skip()
		default:
		}
	}
}

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
