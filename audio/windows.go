package audio

import (
	"slices"
	"unicode/utf16"
	"unsafe"

	win32 "github.com/J-Dufour/maestro/winAPI"
	"golang.org/x/sys/windows"
)

func InitializeAudioAPI() error {
	err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	if err != nil {
		return err
	}

	err = win32.StartMediaFoundation()
	if err != nil {

		return err
	}

	return nil
}

func getDefaultWindowsClient() (winClient *WinAudioClient, err error) {

	winClient = &WinAudioClient{}

	winClient.client, err = win32.GetDefaultClient()
	if err != nil {
		return nil, err
	}

	winClient.format, err = winClient.client.GetMixFormat()
	if err != nil {
		return nil, err
	}

	sharemode := int32(0)
	flags := int32(0)
	hnsBufDuration := int64(100 * 1e6) // 100 ms
	period := 0
	err = winClient.client.Initialize(sharemode, flags, hnsBufDuration, int64(period), winClient.format)
	if err != nil {
		return nil, err
	}

	winClient.renderer, err = winClient.client.GetRenderClient()
	if err != nil {
		return nil, err
	}

	return winClient, nil
}

func wavFormatExToPCMWaveFormat(wav *win32.WaveFormatExtensible) *PCMWaveFormat {
	pcm := &PCMWaveFormat{NumChannels: wav.NChannels, SampleRate: wav.NSamplesPerSec, SampleDepth: wav.WBitsPerSample}
	switch wav.SubFormat {
	case win32.KSDATAFORMAT_SUBTYPE_PCM:
		pcm.PCMType = PCM_TYPE_INT
	case win32.KSDATAFORMAT_SUBTYPE_IEEE_FLOAT:
		pcm.PCMType = PCM_TYPE_FLOAT
	}

	return pcm
}

func PCMWaveFormatToWaveFormatEx(pcm *PCMWaveFormat) *win32.WaveFormatExtensible {
	wav := &win32.WaveFormatExtensible{
		WFormatTag:      win32.WAVE_FORMAT_EXTENSIBLE,
		NChannels:       pcm.NumChannels,
		NSamplesPerSec:  pcm.SampleRate,
		NAvgBytesPerSec: pcm.SampleRate * uint32(pcm.NumChannels) * uint32(pcm.SampleDepth) / 8,
		NBlockAlign:     pcm.NumChannels * pcm.SampleDepth / 8,
		WBitsPerSample:  pcm.SampleDepth,
		CbSize:          22,
		Reserved:        pcm.SampleDepth,
	}

	switch pcm.NumChannels {
	case 1:
		wav.DwChannelMask = win32.SPEAKER_FRONT_CENTER
	case 2:
		wav.DwChannelMask = win32.SPEAKER_FRONT_LEFT | win32.SPEAKER_FRONT_RIGHT
	}

	switch pcm.PCMType {
	case PCM_TYPE_INT:
		wav.SubFormat = win32.KSDATAFORMAT_SUBTYPE_PCM
	case PCM_TYPE_FLOAT:
		wav.SubFormat = win32.KSDATAFORMAT_SUBTYPE_IEEE_FLOAT
	}

	return wav
}

func getWinAudioSourceProvider() *AudioSourceProvider {
	return &AudioSourceProvider{
		createWinAudioSourceFromFile,
	}
}

func createWinAudioSourceFromFile(path string) (AudioSource, error) {
	sourceReader, err := win32.CreateSourceReaderFromFile(path)
	if err != nil {
		return nil, err
	}
	metadata := NewMetadata()
	metadata.Filepath = path
	// grab metadata

	// get media source
	mediaSource, err := sourceReader.GetMediaSource()
	if err != nil {
		return nil, err
	}

	// get metadata provider
	propStore, err := mediaSource.GetPropertyStore()
	if err != nil {
		return &WinAudioSource{metadata, sourceReader}, nil
	}

	// get metadata
	title, err := propStore.GetValue(&win32.PKEY_Title)
	if err == nil {
		metadata.Title = decodeValue(title).(string)
	}

	album, err := propStore.GetValue(&win32.PKEY_Music_AlbumTitle)
	if err == nil {
		metadata.Album = decodeValue(album).(string)
	}

	artist, err := propStore.GetValue(&win32.PKEY_Music_Artist)
	if err == nil {
		metadata.Artist = decodeValue(artist).(string)
	}

	duration, err := propStore.GetValue(&win32.PKEY_Media_Duration)
	if err == nil {
		metadata.Duration = decodeValue(duration).(uint64)
	}

	return &WinAudioSource{metadata, sourceReader}, nil
}

func decodeValue(val win32.PropVariant) any {
	switch val.PropType {
	case win32.VT_BOOL:
		return val.Data == 0xFFFFFFFF
	case win32.VT_UI4:
		return uint32(val.Data)
	case win32.VT_UI8:
		return val.Data
	case win32.VT_LPWSTR:
		return getNullUTF16String((*uint16)(unsafe.Pointer(uintptr(val.Data))))
	default:
		return nil
	}
}

func getNullUTF16String(ptr *uint16) string {
	var str []uint16
	idx := -1
	for i := 1; idx == -1; i++ {
		str = unsafe.Slice(ptr, 32*i)
		idx = slices.Index[[]uint16](str, 0)
	}
	return string(utf16.Decode(str[:idx]))
}

type WinAudioSource struct {
	metadata *Metadata
	reader   *win32.MFSourceReader
}

func (winSource *WinAudioSource) ReadNext() (data []byte, tstamp int, err error) {
	//get sample
	_, _, timestamp, sample, err := winSource.reader.ReadSample(win32.MF_SOURCE_READER_ANY_STREAM, 0)
	if err != nil {
		return nil, 0, err
	}

	//get buffer
	buffer, err := sample.ConvertToContiguousBuffer()
	if err != nil {
		return nil, 0, err
	}

	//return slice
	buffPtr, _, length, err := buffer.Lock()
	if err != nil {
		return nil, 0, err
	}

	data = make([]byte, length)
	copy(data, unsafe.Slice(buffPtr, length))

	buffer.Unlock()
	return data, int(timestamp), nil
}

func (winSource *WinAudioSource) GetPCMWaveFormat() (wav *PCMWaveFormat, err error) {
	waveformatex, err := winSource.reader.GetWaveFormat()
	if err != nil {
		return nil, err
	}
	return wavFormatExToPCMWaveFormat(waveformatex), nil
}

func (winSource *WinAudioSource) SetPCMWaveFormat(wav *PCMWaveFormat) (err error) {
	return winSource.reader.SetWaveFormat(PCMWaveFormatToWaveFormatEx(wav))
}

func (winSource *WinAudioSource) GetMetadata() Metadata {
	return *winSource.metadata
}

type WinAudioClient struct {
	format   *win32.WaveFormatExtensible
	client   *win32.AudioClient
	renderer *win32.AudioRenderClient
}

func (winClient *WinAudioClient) GetPCMWaveFormat() (format *PCMWaveFormat) {
	return wavFormatExToPCMWaveFormat(winClient.format)
}

func (winClient *WinAudioClient) GetBufferSize() (size int, err error) {
	s, err := winClient.client.GetBufferSize()
	size = int(s)
	return size, err
}

func (winClient *WinAudioClient) GetBufferPadding() (padding int, err error) {
	pad, err := winClient.client.GetCurrentPadding()
	padding = int(pad)
	return padding, err
}

func (winClient *WinAudioClient) LoadToBuffer(data []byte) (size int, err error) {
	frameSize := winClient.format.NBlockAlign

	//lock
	buffer, err := winClient.renderer.GetBuffer(uint32(len(data)) / uint32(frameSize))
	if err != nil {
		return 0, err
	}

	//load
	copied := copy(unsafe.Slice(buffer, len(data)), data)

	//release
	err = winClient.renderer.ReleaseBuffer(uint32(copied) / uint32(frameSize))
	if err != nil {
		return 0, err
	}

	return copied, nil
}

func (winClient *WinAudioClient) ClearBuffer() (err error) {
	wasPlaying, err := winClient.client.Stop()
	if err != nil {
		return err
	}

	err = winClient.client.Reset()
	if err != nil {
		return err
	}

	if wasPlaying {
		err = winClient.client.Start()
		if err != nil {
			return err
		}
	}

	return nil
}

func (winClient *WinAudioClient) Start() (err error) {
	return winClient.client.Start()
}

func (winClient *WinAudioClient) Stop() (wasPlaying bool, err error) {
	return winClient.client.Stop()
}
