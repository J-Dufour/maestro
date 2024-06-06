package audio

import (
	"io"
	"time"
)

const (
	EVENT_SOURCE_CHANGE = iota
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

	Duration uint64
}

func NewMetadata() (m *Metadata) {
	m = &Metadata{}
	m.Filepath = NOT_FOUND
	m.Title = NOT_FOUND
	m.Artist = NOT_FOUND
	m.Duration = 0
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
	ReadNext() ([]byte, int, error)
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

	trackPosition int // in 100ns units

	queueIn  chan AudioSource
	queue    []AudioSource
	queueIdx int

	sourceChangeSubscribers []chan<- struct{}
	queueUpdateSubscribers  []chan<- struct{}
}

func getDefaultClient() (AudioClient, error) {
	return getDefaultWindowsClient()
}

func GetAudioSourceProvider() *AudioSourceProvider {
	return getWinAudioSourceProvider()
}

func NewPlayer() (player *Player, err error) {
	player = &Player{}

	player.control = make(chan int, 16)
	player.queueIn = make(chan AudioSource, 2)
	player.queue = make([]AudioSource, 0)
	player.queueIdx = 0

	player.queueUpdateSubscribers = make([]chan<- struct{}, 0)
	player.sourceChangeSubscribers = make([]chan<- struct{}, 0)

	// make player thread
	client, err := getDefaultClient()
	player.client = client
	player.format = client.GetPCMWaveFormat()
	if err != nil {
		return nil, err
	}
	go player.playerThread()

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

func (p *Player) AddSourcesToQueue(sources ...AudioSource) {
	for _, source := range sources {
		source.SetPCMWaveFormat(p.format)
		p.queueIn <- source
	}
}

func (p *Player) GetQueue() []AudioSource {
	return append(make([]AudioSource, 0, len(p.queue)), p.queue...)
}

func (p *Player) GetPositionInQueue() int {
	return p.queueIdx
}

func (p *Player) GetPositionInTrack() int {
	return p.trackPosition
}

func (p *Player) SubscribeToSourceChange(c chan<- struct{}) {
	p.sourceChangeSubscribers = append(p.sourceChangeSubscribers, c)
}

func (p *Player) publishSourceChange() {
	for _, c := range p.sourceChangeSubscribers {
		c <- struct{}{}
	}
}

func (p *Player) SubscribeToQueueUpdate(c chan<- struct{}) {
	p.queueUpdateSubscribers = append(p.queueUpdateSubscribers, c)
}

func (p *Player) publishQueueUpdate() {
	for _, c := range p.queueUpdateSubscribers {
		c <- struct{}{}
	}
}

func (player *Player) playerThread() {
	CLK_DUR := 100 * time.Millisecond

	client := player.client
	format := client.GetPCMWaveFormat()

	//initialize audio queue
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

	// last known timestamp
	lastKnownTS := 0

	// if EOF is reached
	reachedEOF := false

	bytesTo100ns := (8 * 1e7) / (int(format.SampleDepth) * int(format.SampleRate) * int(format.NumChannels))

	for {
		select {
		case source := <-player.queueIn:
			player.queue = append(player.queue, source)
			player.publishQueueUpdate()
			if waitingForNextTrack {
				curSource = player.queue[player.queueIdx]
				player.publishSourceChange()
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

				if amt == 0 || player.queueIdx >= len(player.queue) { //if skipping 0 songs, or if index is already waiting, skip
					break
				}
				player.queueIdx += amt
				player.queueIdx = Clamp(player.queueIdx, 0, len(player.queue))
				player.publishSourceChange()
				if player.queueIdx < len(player.queue) {
					waitingForNextTrack = false

					// interrupt current song
					leftover = leftover[:0]
					client.ClearBuffer()
					// play next song
					curSource = player.queue[player.queueIdx]
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

			// Estimate timestamp
			totalBufferedData := (padding * frameSize) + len(leftover)
			timeDiff := totalBufferedData * bytesTo100ns
			player.trackPosition = lastKnownTS - timeDiff

			if reachedEOF && player.trackPosition == lastKnownTS { //if song is done
				reachedEOF = false
				// exit and move to next song
				player.queueIdx++
				if player.queueIdx < len(player.queue) {
					curSource = player.queue[player.queueIdx]
				} else { //if no next song, wait.
					waitingForNextTrack = true
					clock.Stop()
					player.publishSourceChange()
					break
				}
				lastKnownTS = 0
				player.publishSourceChange()

			}

			freeFrames := bufferFrames - padding

			// initialize accumulator
			acc := make([]byte, freeFrames*frameSize)

			//load leftover
			totalCopied := copy(acc, leftover)
			leftover = leftover[totalCopied:]

			//load new data
			for i := 0; totalCopied < freeFrames*frameSize; i++ {
				frames, timestamp, err := curSource.ReadNext()
				if err == io.EOF {
					reachedEOF = true
					break
				} else if err != nil {
					panic(err)
				}
				copied := copy(acc[totalCopied:], frames)
				if copied < len(frames) {
					leftover = frames[copied:]
				}
				totalCopied += copied
				lastKnownTS = timestamp + len(frames)*bytesTo100ns
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
