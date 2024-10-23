package audio

import (
	"errors"
	"io"
	"time"
)

const (
	SECOND         = 1e7
	BACK_THRESHOLD = 2 * SECOND // in 100ns units
	SEEK_UNIT      = 5 * SECOND
)

const (
	EVENT_SOURCE_CHANGE = iota
)

const (
	CTL_PLAY = iota
	CTL_PAUSE
	CTL_SKIP
	CTL_SEEK
	CTL_SEEK_TO
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
	Album  string
	Artist string

	Duration uint64
}

func NewMetadata() (m *Metadata) {
	m = &Metadata{}
	m.Filepath = NOT_FOUND
	m.Title = NOT_FOUND
	m.Artist = NOT_FOUND
	m.Album = NOT_FOUND
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
	SetPosition(int64) error

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

	control     chan int
	controlDone chan struct{}
	playing     bool

	trackPosition int // in 100ns units

	queueIn chan string
	queue   Queue

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

	player.control = make(chan int)
	player.controlDone = make(chan struct{})
	player.playing = false

	player.queueIn = make(chan string, 2)
	player.queue = Queue{make([]QueueItem, 0), -1}

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
	<-p.controlDone
}

func (p *Player) Stop() {
	p.control <- CTL_PAUSE
	<-p.controlDone
}

func (p *Player) Toggle() {
	if p.playing {
		p.Stop()
	} else {
		p.Start()
	}
}

func (p *Player) Skip() {
	p.control <- CTL_SKIP
	p.control <- 1
	<-p.controlDone

}

func (p *Player) Back() {
	if p.trackPosition < BACK_THRESHOLD {
		// skip backwards
		p.control <- CTL_SKIP
		p.control <- -1
	} else {
		// restart the song
		p.control <- CTL_SEEK_TO
		p.control <- 0
	}

	<-p.controlDone
}

func (p *Player) SeekForward() {
	p.control <- CTL_SEEK
	p.control <- SEEK_UNIT
	<-p.controlDone
}

func (p *Player) SeekBackward() {
	p.control <- CTL_SEEK
	p.control <- -1 * SEEK_UNIT
	<-p.controlDone
}

func (p *Player) AddSourcesToQueue(sources ...string) {
	for _, source := range sources {
		p.queueIn <- source
	}
}

func (p *Player) GetQueue() []Metadata {
	return p.queue.GetDataQueue()
}

func (p *Player) GetPositionInQueue() int {
	return p.queue.idx
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
			player.queue.AddSourcePath(source, player.format)
			player.publishQueueUpdate()
			if waitingForNextTrack {
				curSource, waitingForNextTrack = player.queue.NextSource()
				if waitingForNextTrack {
					break
				}

				player.trackPosition = 0
				lastKnownTS = 0

				player.publishSourceChange()
				clock.Reset(CLK_DUR)
			}
		case op := <-player.control:
			switch op {
			case CTL_PLAY:
				client.Start()
				player.playing = true
				player.controlDone <- struct{}{}
			case CTL_PAUSE:
				client.Stop()
				player.playing = false
				player.controlDone <- struct{}{}
			case CTL_SKIP:
				//grab exra data
				amt := <-player.control

				if amt == 0 { //if skipping 0 songs, or if index is already waiting, skip
					player.controlDone <- struct{}{}
					break
				}

				player.queue.SkipIdx(amt)

				// interrupt current song
				leftover = leftover[:0]
				client.ClearBuffer()

				curSource.SetPosition(0)
				curSource, waitingForNextTrack = player.queue.NextSource()

				if waitingForNextTrack {
					curSource.SetPosition(int64(curSource.GetMetadata().Duration))
					player.trackPosition = int(curSource.GetMetadata().Duration)
					clock.Stop()
				} else {
					// play next song
					player.trackPosition = 0
					lastKnownTS = 0
					clock.Reset(CLK_DUR)
				}

				player.publishSourceChange()
				player.controlDone <- struct{}{}
			case CTL_SEEK:
				// find new position
				amt := <-player.control
				newPos := player.trackPosition + amt
				newPos = Clamp(newPos, 0, int(curSource.GetMetadata().Duration))

				// set new position
				curSource.SetPosition(int64(newPos))
				player.trackPosition = newPos
				lastKnownTS = player.trackPosition

				// clear buffer
				player.client.ClearBuffer()
				leftover = leftover[:0]

				player.controlDone <- struct{}{}

			case CTL_SEEK_TO:
				curSource.SetPosition(int64(<-player.control))
				client.ClearBuffer()
				leftover = leftover[:0]
				player.controlDone <- struct{}{}
			}
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
				curSource.SetPosition(0)
				curSource, waitingForNextTrack = player.queue.NextSource()

				player.trackPosition = 0
				lastKnownTS = 0
				if waitingForNextTrack {
					clock.Stop()
					player.publishSourceChange()
					break
				}
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
			if totalCopied > 0 {
				//load into buffer
				client.LoadToBuffer(acc[:totalCopied])
			}

		}
	}
}

type Queue struct {
	queue []QueueItem
	idx   int
}

func (q *Queue) AddSourcePath(path string, format *PCMWaveFormat) {
	metadata, err := WinGetFileMetadata(path)
	if err != nil {
		metadata := NewMetadata()
		metadata.Filepath = path
	}
	q.queue = append(q.queue, QueueItem{*metadata, format, nil})
}

func (q *Queue) NextSource() (s AudioSource, endOfQueue bool) {
	// find next valid source
	err := errors.New("test")
	var source AudioSource
	for err != nil && q.idx < len(q.queue) {
		q.idx++
		source, err = q.queue[q.idx].Source()
	}
	if q.idx == len(q.queue) {
		return nil, true
	}
	return source, false
}

// moves idx such that the next song is [amt] away from current song
func (q *Queue) SkipIdx(amt int) {
	q.idx = Clamp(q.idx+amt-1, -1, len(q.queue))
}

func (q *Queue) GetDataQueue() []Metadata {
	out := make([]Metadata, len(q.queue))
	for i, item := range q.queue {
		out[i] = item.metadata
	}

	return out
}

type QueueItem struct {
	metadata Metadata
	format   *PCMWaveFormat
	source   AudioSource
}

func (i *QueueItem) Source() (AudioSource, error) {
	if i.source == nil {
		err := i.loadSource()
		if err != nil {
			return nil, err
		}
	}

	return i.source, nil
}

func (i *QueueItem) loadSource() error {
	s, err := GetAudioSourceProvider().GetAudioSourceFromFile(i.metadata.Filepath)
	if err != nil {
		return err
	}

	if i.format != nil {
		err = s.SetPCMWaveFormat(i.format)
		if err != nil {
			return err
		}
	}

	i.source = s
	return nil
}

func Clamp(x int, min int, max int) int {
	if x < min {
		x = min
	} else if x > max {
		x = max
	}

	return x
}
