package audio

import (
	"time"
)

const (
	PCM_TYPE_INT   = iota
	PCM_TYPE_FLOAT = iota
)

type PCMWaveFormat struct {
	NumChannels uint16
	SampleRate  uint32
	SampleDepth uint16
	PCMType     uint32
}

type AudioClient interface {
	GetPCMWaveFormat() *PCMWaveFormat

	GetBufferSize() (int, error)
	GetBufferPadding() (int, error)
	LoadToBuffer([]byte) (int, error)
	ClearBuffer() error

	Start() error
	Stop() (bool, error)
}

type AudioSource interface {
	ReadNext() ([]byte, error)
	SetPCMWaveFormat(*PCMWaveFormat) error
	GetPCMWaveFormat() (*PCMWaveFormat, error)
}

type AudioSourceProvider struct {
	GetAudioSourceFromFile func(filepath string) (AudioSource, error)
}

type Player struct {
	client AudioClient
	format *PCMWaveFormat

	playerData  chan []byte
	clearPlayer chan int

	skip chan int

	queue chan *Reader
}

func getDefaultClient() (AudioClient, error) {
	return getDefaultWindowsClient()
}

func GetAudioSourceProvider() *AudioSourceProvider {
	return getWinAudioSourceProvider()
}

func NewPlayer(sources ...AudioSource) (player *Player, err error) {
	player = &Player{}

	player.playerData = make(chan []byte)
	player.clearPlayer = make(chan int)
	player.skip = make(chan int)
	player.queue = make(chan *Reader, 2)

	// make player thread
	client, err := getDefaultClient()
	player.client = client
	player.format = client.GetPCMWaveFormat()
	if err != nil {
		return nil, err
	}
	go musicPlayer(player.client, player.playerData, player.clearPlayer)
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

func (p *Player) AddSourceToQueue(s AudioSource) {
	reader := NewReader(s)
	reader.SetWaveFormat(p.format)
	p.queue <- reader
}

type Reader struct {
	source AudioSource
}

func NewReader(s AudioSource) (reader *Reader) {
	reader = &Reader{}
	reader.source = s
	return reader
}

func (r *Reader) SetWaveFormat(wav *PCMWaveFormat) error {
	return r.source.SetPCMWaveFormat(wav)
}

func musicPlayer(client AudioClient, data chan []byte, clear chan int) {
	format := client.GetPCMWaveFormat()

	// get max buffer size
	bufferFrames, err := client.GetBufferSize()
	if err != nil {
		panic(err)
	}

	// get frame size
	frameSize := int(format.NumChannels * format.SampleDepth / 8)

	// create clock
	clock := time.NewTicker(100 * time.Millisecond)

	//

	for {
		select {
		case <-clear:
			client.ClearBuffer()
		case <-clock.C:
			// Get buffer
			padding, err := client.GetBufferPadding()
			if err != nil {
				panic(err)
			}
			freeFrames := bufferFrames - padding

			// initialize accumulator
			acc := make([]byte, freeFrames*frameSize)

			// load until full or nothing in channel
			total := 0

			dataAvailable := true
			for i := 0; i < int(freeFrames) && dataAvailable; i++ {
				select {
				case frame := <-data:
					total += copy(acc[i*frameSize:], frame)
				case <-time.After(time.Millisecond):
					acc = acc[:total]
					dataAvailable = false
				}
			}

			//load into buffer
			client.LoadToBuffer(acc)

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
				//play song
				go musicReader(queue[idx].source, data, clearChan, quitChan, skip)
				waitingForNextTrack = false
			}
		case num := <-skip:
			if num == 0 || idx >= len(queue) { //if skipping 0 songs, or if index is already waiting, skip
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

func musicReader(source AudioSource, dataBuf chan []byte, clear chan int, quit chan int, done chan int) {
	format, _ := source.GetPCMWaveFormat()
	frameSize := format.NumChannels * format.SampleDepth / 8
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
