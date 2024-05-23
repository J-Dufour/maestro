package main

import (
	"fmt"
	"os"

	"github.com/J-Dufour/maestro/audio"
)

func main() {

	// startup
	audio.InitializeAudioAPI()

	// get file reader
	if len(os.Args) < 2 {
		fmt.Println("please provide path to valid music file")
		return
	}
	sourceProvider := audio.GetAudioSourceProvider()

	sources := make([]audio.AudioSource, 0)
	for _, fileName := range os.Args[1:] {
		source, err := sourceProvider.GetAudioSourceFromFile(fileName)
		if err != nil {
			fmt.Println(err)
			return
		}

		sources = append(sources, source)
	}

	player, err := audio.NewPlayer(sources...)
	if err != nil {
		fmt.Println(err)
		return
	}

	player.Start()

	// start UI
	root, initLoop, input := InitTerminalLoop(20)

	// start input interpreter
	go inputDecoder(input, player)

	// make static queue view
	maxLength := len(os.Args[1])
	for _, arg := range os.Args[2:] {
		if maxLength < len(arg) {
			maxLength = len(arg)
		}
	}
	queueWin := root.NewChild(Box{0, 0, uint(7 + maxLength), uint(4 + len(os.Args[1:]))})

	queueWin.Exec(queueWin.DrawBox(Box{0, 0, queueWin.w, queueWin.h}, " Queue "))
	listCom := queueWin.GetOffsetComBuilder().Offset(1, 1)
	for i, name := range os.Args[1:] {
		listCom.MoveLines(1).Offset(2, 0).Write(i+1, ". ", name)
	}

	queueWin.Exec(listCom.BuildCom())

	initLoop()
}

func inputDecoder(input chan byte, player *audio.Player) {
	for key := range input {
		switch key {
		case 'k':
			player.Skip()
		default:
		}
	}
}
