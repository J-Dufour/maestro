package main

import (
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type SoundSource interface {
	ReadNext() ([]byte, error)
	SetWaveFormat(format *WaveFormatExtensible) error
	GetWaveFormat() (*WaveFormatExtensible, error)
}

type Player struct {
	client *AudioClient
	format *WaveFormatExtensible

	playerData  chan []byte
	clearPlayer chan int

	skip chan int

	queue chan *Reader
}

func NewPlayer(sources ...SoundSource) (player *Player, err error) {
	player = &Player{}

	player.playerData = make(chan []byte)
	player.clearPlayer = make(chan int)
	player.skip = make(chan int)
	player.queue = make(chan *Reader, 2)

	// make player thread
	client, format, err := initDefaultClient()
	player.client = client
	player.format = format
	if err != nil {
		return nil, err
	}
	go musicPlayer(player.client, format, player.playerData, player.clearPlayer)
	go sequencer(player.queue, player.playerData, player.skip)
	// add sources to queue
	for _, source := range sources {
		player.AddSourceToQueue(source)
	}

	return player, nil
}

func (p *Player) Start() {
	p.client.Start()
}

func (p *Player) Stop() {
	p.client.Stop()
}

func (p *Player) Skip() {
	p.skip <- 1
	p.clearPlayer <- 1
}

func (p *Player) AddSourceToQueue(s SoundSource) {
	reader := NewReader(s)
	reader.SetWaveFormat(p.format)
	p.queue <- reader
}

type Reader struct {
	source SoundSource
}

func NewReader(s SoundSource) (reader *Reader) {
	reader = &Reader{}
	reader.source = s
	return reader
}

func (r *Reader) SetWaveFormat(wav *WaveFormatExtensible) {
	r.source.SetWaveFormat(wav)
}

func main() {

	// startup
	err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	if err != nil {
		fmt.Println(err)
		return
	}

	err = StartMediaFoundation()
	if err != nil {
		fmt.Println(err)
		return
	}

	// get file reader
	if len(os.Args) < 2 {
		fmt.Println("please provide path to valid music file")
		return
	}

	sources := make([]SoundSource, 0)
	for _, fileName := range os.Args[1:] {
		source, err := GetSourceReaderFromFile(fileName)
		if err != nil {
			fmt.Println(err)
			return
		}

		sources = append(sources, source)
	}

	player, err := NewPlayer(sources...)
	if err != nil {
		fmt.Println(err)
		return
	}

	player.Start()

	// start UI
	comChan := make(chan Com, 3)
	root, initLoop, input := InitTerminalLoop(20, comChan)

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

	comChan <- queueWin.DrawBox(Box{0, 0, queueWin.w, queueWin.h}, " Queue ")
	listCom := queueWin.GetOffsetComBuilder().Offset(1, 1)
	for i, name := range os.Args[1:] {
		listCom.MoveLines(1).Offset(2, 0).Write(i+1, ". ", name)
	}

	comChan <- listCom.BuildCom()

	initLoop()
}

func inputDecoder(input chan byte, player *Player) {
	for key := range input {
		switch key {
		case 'k':
			player.Skip()
		default:
		}
	}
}

func initDefaultClient() (client *AudioClient, format *WaveFormatExtensible, err error) {
	client, err = GetDefaultClient()
	if err != nil {
		return nil, nil, err
	}

	// get format
	format, err = client.getMixFormat()
	if err != nil {
		return nil, nil, err
	}

	sharemode := int32(0)
	flags := int32(0)
	hnsBufDuration := int64(100 * 1e6) // 100 ms
	period := 0
	err = client.Initialize(sharemode, flags, hnsBufDuration, int64(period), format)
	if err != nil {
		return nil, nil, err
	}

	return client, format, nil
}

func (s MFSourceReader) ReadNext() (data []byte, err error) {
	//get sample
	_, _, _, sample, err := s.ReadSample(MF_SOURCE_READER_ANY_STREAM, 0)
	if err != nil {
		return nil, err
	}

	//get buffer
	buffer, err := sample.ConvertToContiguousBuffer()
	if err != nil {
		return nil, err
	}

	//return slice
	buffPtr, _, length, err := buffer.Lock()
	if err != nil {
		return nil, err
	}

	data = make([]byte, length)
	copy(data, unsafe.Slice(buffPtr, length))

	buffer.Unlock()
	return data, nil
}

func musicPlayer(client *AudioClient, format *WaveFormatExtensible, data chan []byte, clear chan int) (err error) {
	// get render client
	renderClient, err := client.GetRenderClient()
	if err != nil {
		fmt.Println(err)
		return
	}

	// get max buffer size
	bufferFrames, err := client.GetBufferSize()
	if err != nil {
		fmt.Println(err)
		return
	}

	// get frame size
	frameSize := int(format.nBlockAlign)

	// create clock
	clock := time.NewTicker(100 * time.Millisecond)

	//

	for {
		select {
		case <-clear:
			client.Stop()
			client.Reset()
			client.Start()
		case <-clock.C:
			// Get buffer
			padding, err := client.GetCurrentPadding()
			if err != nil {
				fmt.Println(err)
				return nil
			}

			freeFrames := bufferFrames - padding

			buff, err := renderClient.GetBuffer(freeFrames)
			if err != nil {
				fmt.Println(err)
				return nil
			}

			// initialize buffer
			freeData := freeFrames * uint32(frameSize)
			acc := make([]byte, freeData)

			// load until full or nothing in channel
			total := 0
			i := 0
			dataAvailable := true
			for i < int(freeFrames) && dataAvailable {
				select {
				case frame := <-data:
					total += copy(acc[i*frameSize:], frame)
				case <-time.After(time.Millisecond):
					acc = acc[:total]
					dataAvailable = false
				}
				i++
			}

			// copy accumulated data into real buffer
			copy(unsafe.Slice(buff, len(acc)), acc)

			// release buffer
			err = renderClient.ReleaseBuffer(uint32(len(acc) / frameSize))
			if err != nil {
				fmt.Println(err)
				return nil
			}
		}
	}
}

func sequencer(addSource chan *Reader, data chan []byte, skip chan int) {
	waitingForNextTrack := true
	queue := make([]*Reader, 0)
	idx := 0

	clearChan := make(chan int)
	quitChan := make(chan int)
	for {
		select {
		case reader := <-addSource:
			queue = append(queue, reader)
			if waitingForNextTrack {
				fmt.Println(queue)
				//play song
				go musicReader(queue[idx].source, data, clearChan, quitChan, skip)
				waitingForNextTrack = false
			}
		case num := <-skip:
			if num == 0 {
				break
			}
			idx += num
			idx = Clamp(idx, 0, len(queue))
			if idx < len(queue) {
				waitingForNextTrack = false
				//interrupt current song
				quitChan <- 1
				//play song
				go musicReader(queue[idx].source, data, clearChan, quitChan, skip)

			} else {
				//interrupt current song
				quitChan <- 1
				waitingForNextTrack = true
			}

		}
	}
}

func musicReader(source SoundSource, dataBuf chan []byte, clear chan int, quit chan int, done chan int) {
	format, _ := source.GetWaveFormat()
	frameSize := format.nBlockAlign
	keepLoading := true
	for {
		data, err := source.ReadNext()
		if err != nil {
			done <- 1
			<-quit
			return
		}
		keepLoading = true
		for len(data) > 0 && keepLoading {
			select {
			case <-clear:
				keepLoading = false
			case <-quit:
				return
			case dataBuf <- data[:frameSize]:

				data = data[frameSize:]
			}
		}
	}

}

func Clamp(x int, min int, max int) int {
	if x < min {
		x = min
	} else if x > max {
		x = max
	}

	return x
}
