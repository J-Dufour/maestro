package main

import (
	"fmt"
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
	root, done, input := InitTerminalLoop()

	// start input interpreter
	go inputDecoder(input, player)

	// make queue view
	outerQueueWin := root.NewChild(Box{0, 0, 40, 14})
	NewOuterWindowController(outerQueueWin, " Queue ")
	innerQueueWin := outerQueueWin.NewChild(Box{1, 1, 38, 12})

	Qcontroller := NewQueueWindowController(innerQueueWin)
	StartQueueWindowLoop(Qcontroller, player)

	// make player view
	outerPlayerWin := root.NewChild(Box{41, 0, 40, 14})
	NewOuterWindowController(outerPlayerWin, " Player ")
	innerPlayerWin := outerPlayerWin.NewChild(Box{1, 1, 38, 12})

	Pcontroller := NewPlayerWindowController(innerPlayerWin)
	StartPlayerWindowLoop(Pcontroller, player)

	player.AddSourcesToQueue(sources...)
	player.Start()

	<-done
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
