package audio

import (
	"io"
	"time"
)

const (
	CTL_PLAY = iota
	CTL_PAUSE
	CTL_SKIP
)

const (
	PCM_TYPE_INT = iota
	PCM_TYPE_FLOAT
)

const (
	NOT_FOUND = "Unknown"
)

type PCMWaveFormat struct {
	NumChannels uint16
	SampleRate  uint32
	SampleDepth uint16
	PCMType     uint32
}

type Metadata struct {
	Filepath string

	Title  string
	Artist string
}

func NewMetadata() (m *Metadata) {
	m = &Metadata{}
	m.Filepath = NOT_FOUND
	m.Title = NOT_FOUND

	return m
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

	GetMetadata() Metadata
}

type AudioSourceProvider struct {
	GetAudioSourceFromFile func(filepath string) (AudioSource, error)
}

type Player struct {
	client AudioClient
	format *PCMWaveFormat

	control chan int

	queueIn chan AudioSource
}

func getDefaultClient() (AudioClient, error) {
	return getDefaultWindowsClient()
}

func GetAudioSourceProvider() *AudioSourceProvider {
	return getWinAudioSourceProvider()
}

func NewPlayer(sources ...AudioSource) (player *Player, err error) {
	player = &Player{}

	player.control = make(chan int, 16)
	player.queueIn = make(chan AudioSource, 2)

	// make player thread
	client, err := getDefaultClient()
	player.client = client
	player.format = client.GetPCMWaveFormat()
	if err != nil {
		return nil, err
	}
	go player.playerThread()

	// add sources to queue
	for _, source := range sources {
		player.AddSourceToQueue(source)
	}

	return player, nil
}

func (p *Player) Start() {
	p.control <- CTL_PLAY
}

func (p *Player) Stop() {
	p.control <- CTL_PAUSE
}

func (p *Player) Skip() {
	p.control <- CTL_SKIP
	p.control <- 1

}

func (p *Player) AddSourceToQueue(s AudioSource) {
	s.SetPCMWaveFormat(p.format)
	p.queueIn <- s
}

func (player *Player) playerThread() {
	CLK_DUR := 100 * time.Millisecond

	client := player.client
	format := client.GetPCMWaveFormat()

	//initialize audio queue
	queue := make([]AudioSource, 0)
	idx := 0
	var curSource AudioSource
	waitingForNextTrack := true

	// get max buffer size
	bufferFrames, err := client.GetBufferSize()
	if err != nil {
		panic(err)
	}

	// get frame size
	frameSize := int(format.NumChannels * format.SampleDepth / 8)

	//initialize "leftover" buffer
	leftover := make([]byte, 0)

	// create clock
	clock := time.NewTicker(CLK_DUR)
	clock.Stop() // wait for first track
	//

	for {
		select {
		case source := <-player.queueIn:
			queue = append(queue, source)
			if waitingForNextTrack {
				curSource = queue[idx]
				clock.Reset(CLK_DUR)
				waitingForNextTrack = false
			}
		case op := <-player.control:
			switch op {
			case CTL_PLAY:
				client.Start()
			case CTL_PAUSE:
				client.Stop()
			case CTL_SKIP:
				//grab exra data
				amt := <-player.control

				if amt == 0 || idx >= len(queue) { //if skipping 0 songs, or if index is already waiting, skip
					break
				}
				idx += amt
				idx = Clamp(idx, 0, len(queue))
				if idx < len(queue) {
					waitingForNextTrack = false

					// interrupt current song
					leftover = leftover[:0]
					client.ClearBuffer()
					// play next song
					curSource = queue[idx]
					clock.Reset(CLK_DUR)
				} else {
					// wait for next song
					waitingForNextTrack = true
					clock.Stop()
				}

			}
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

			//load leftover
			totalCopied := copy(acc, leftover)
			leftover = leftover[totalCopied:]

			//load new data
			for i := 0; totalCopied < freeFrames*frameSize; i++ {
				frames, err := curSource.ReadNext()
				if err == io.EOF {
					// exit and move to next song
					idx++
					if idx < len(queue) {
						curSource = queue[idx]
					} else { //if no next song, wait.
						waitingForNextTrack = true
						clock.Stop()
					}
					break

				} else if err != nil {
					panic(err)
				}
				copied := copy(acc[totalCopied:], frames)
				if copied < len(frames) {
					leftover = frames[copied:]
				}
				totalCopied += copied
			}

			//load into buffer
			client.LoadToBuffer(acc[:totalCopied])

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
