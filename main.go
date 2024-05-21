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
}

type Player struct {
	client *AudioClient

	data    chan []byte
	control chan int

	queueTail chan int

	Done chan int
}

func NewPlayer(sources ...SoundSource) (player *Player, err error) {
	player = &Player{}
	player.data = make(chan []byte, 10)
	player.control = make(chan int)
	player.queueTail = make(chan int, 1)
	player.queueTail <- 1
	player.Done = make(chan int)

	// make player thread
	client, format, err := initDefaultClient()
	player.client = client
	if err != nil {
		return nil, err
	}
	go musicPlayer(player.client, format, player.data, player.control, player.Done)

	// add sources to queue
	for _, source := range sources {
		source.SetWaveFormat(format)
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

func (p *Player) AddSourceToQueue(s SoundSource) {
	newTail := make(chan int)
	go musicReader(s, p.data, p.queueTail, newTail)
	p.queueTail = newTail
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

	//start UI
	comChan := make(chan Com, 3)
	root, initLoop := InitTerminalLoop(20, comChan)

	player.Start()

	//make static queue view
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

func musicPlayer(client *AudioClient, format *WaveFormatExtensible, dataBuf chan []byte, control chan int, done chan int) (err error) {
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

	//get frame size
	frameSize := int(format.nBlockAlign)

	// create leftover data
	leftover := make([]byte, 0)

	// create clock
	clock := time.NewTicker(100 * time.Millisecond)

	dataChanClosed := false
	for len(leftover) > 0 || !dataChanClosed {
		select {
		case op := <-control:
			if op == 0 {
				break
			}
		case <-clock.C:
			//Get buffer
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

			//load leftover data into accumulator
			freeData := freeFrames * uint32(frameSize)
			acc := make([]byte, freeData)
			copy(acc, leftover)

			leftoverLen := len(leftover)

			// if buffer can fit more data, grab from channel
			if leftoverLen < int(freeData) {
				leftover = leftover[:0]
				freeData -= uint32(leftoverLen)
				moreData := true

				// load until full or nothing in channel
				for moreData {
					select {
					case frames, open := <-dataBuf:
						dataChanClosed = !open
						copied := copy(acc[len(acc)-int(freeData):], frames)
						freeData -= uint32(copied)
						//buffer full
						if copied < len(frames) {
							leftover = frames[copied:]
							moreData = false
						}

						if dataChanClosed {
							moreData = false
						}
					default:
						moreData = false // if no data, move on
						//shrink acc to acutal data
						totalCopied := len(acc) - int(freeData)
						acc = acc[:totalCopied]
					}
				}
			} else {
				leftover = leftover[freeData:]
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

	// get remaining buffer and wait until end
	padding, err := client.GetCurrentPadding()
	if err != nil {
		//if somehow fails, just assume the buffer is full
		padding = bufferFrames
	}

	time.Sleep(time.Duration(padding*uint32(frameSize)/(format.nAvgBytesPerSec)) * time.Second)
	done <- 1
	return nil
}

func musicReader(source SoundSource, dataBuf chan []byte, start chan int, done chan int) {
	<-start
	for {
		data, err := source.ReadNext()
		if err != nil {
			done <- 1
			return
		}

		dataBuf <- data
	}

}
