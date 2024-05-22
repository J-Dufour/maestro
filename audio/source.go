package audio

import win32 "github.com/J-Dufour/maestro/winAPI"

type SoundSource interface {
	ReadNext() ([]byte, error)
	SetWaveFormat(format *win32.WaveFormatExtensible) error
	GetWaveFormat() (*win32.WaveFormatExtensible, error)
}

func GetSoundSourceFromFile(fileName string) (source SoundSource, err error) {
	return win32.GetSourceReaderFromFile(fileName)
}
