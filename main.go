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

	reader, err := GetSourceReaderFromFile(os.Args[1])
	if err != nil {
		fmt.Println(err)
		return
	}

	// get audio client
	client, format, err := initDefaultClient()
	if err != nil {
		fmt.Println(err)
		return
	}

	reader.SetWaveFormat(format)

	// make channels
	dataChan := make(chan []byte, 10)
	controlChan := make(chan int)

	startReader := make(chan int, 1)
	readerDone := make(chan int)
	playerDone := make(chan int)
	// start goroutines
	startReader <- 1 // start reader immediately
	client.Start()
	go musicReader(reader, dataChan, startReader, readerDone)
	go musicPlayer(client, format, dataChan, controlChan, playerDone)
	<-readerDone    // wait until reader finishes
	close(dataChan) // close data channel
	<-playerDone    //wait until player finishes
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
