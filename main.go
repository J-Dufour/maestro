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
	KEY_QUIT   = 'q'
	KEY_SKIP   = 'k'
	KEY_TOGGLE = ' '
	KEY_BACK   = 'j'
	KEY_SEEKF  = '.'
	KEY_SEEKB  = ','
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

	player, err := audio.NewPlayer()
	if err != nil {
		fmt.Println(err)
		return
	}

	// start UI
	outerPWin, done, input := InitTerminalLoop()

	// start input interpreter
	go inputDecoder(input, player)

	// split
	outerPWin.VSplit()
	outerQWin := outerPWin.HSplit()

	// make queue view
	outerQWin.SetController(QueueWindowControllerFunc(player))

	// make player view
	outerPWin.SetController(PlayerWindowControllerFunc(player))

	player.AddSourcesToQueue(absolutePaths...)
	player.Start()

	<-done
}

func inputDecoder(input chan byte, player *audio.Player) {
	for key := range input {
		switch key {
		case KEY_SKIP:
			player.Skip()
		case KEY_TOGGLE:
			player.Toggle()
		case KEY_BACK:
			player.Back()
		case KEY_SEEKF:
			player.SeekForward()
		case KEY_SEEKB:
			player.SeekBackward()
		default:
		}
	}
}
