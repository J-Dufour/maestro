package winAPI

import (
	"errors"
	"fmt"
	"io"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	Mfplat      = windows.NewLazyDLL("Mfplat.dll")
	Mfreadwrite = windows.NewLazyDLL("Mfreadwrite.dll")
	Mf          = windows.NewLazyDLL("Mf.dll")

	MFStartup                           = Mfplat.NewProc("MFStartup")
	MFCreateMediaType                   = Mfplat.NewProc("MFCreateMediaType")
	MFCreateWaveFormatExFromMFMediaType = Mfplat.NewProc("MFCreateWaveFormatExFromMFMediaType")
	MFCreateAudioMediaType              = Mfplat.NewProc("MFCreateAudioMediaType")

	MFCreateSourceReaderFromURL     = Mfreadwrite.NewProc("MFCreateSourceReaderFromURL")
	MFCreateSinkWriterFromMediaSink = Mfreadwrite.NewProc("MFCreateSinkWriterFromMediaSink")

	MFCreateAudioRenderer = Mf.NewProc("MFCreateAudioRenderer")
	MFGetService          = Mf.NewProc("MFGetService")
)

var (
	MF_MT_MAJOR_TYPE                  = windows.GUID{Data1: 0x48eba18e, Data2: 0xf8c9, Data3: 0x4687, Data4: [8]byte{0xbf, 0x11, 0x0a, 0x74, 0xc9, 0xf9, 0x6a, 0x8f}}
	MF_MT_SUBTYPE                     = windows.GUID{Data1: 0xf7e34c9a, Data2: 0x42e8, Data3: 0x4714, Data4: [8]byte{0xb7, 0x4b, 0xcb, 0x29, 0xd7, 0x2c, 0x35, 0xe5}}
	MF_MT_AUDIO_NUM_CHANNELS          = windows.GUID{Data1: 0x37e48bf5, Data2: 0x645e, Data3: 0x4c5b, Data4: [8]byte{0x89, 0xde, 0xad, 0xa9, 0xe2, 0x9b, 0x69, 0x6a}}
	MF_MT_AUDIO_SAMPLES_PER_SECOND    = windows.GUID{Data1: 0x5faeeae7, Data2: 0x0290, Data3: 0x4c31, Data4: [8]byte{0x9e, 0x8a, 0xc5, 0x34, 0xf6, 0x8d, 0x9d, 0xba}}
	MF_MT_AUDIO_AVG_BYTES_PER_SECOND  = windows.GUID{Data1: 0x1aab75c8, Data2: 0xcfef, Data3: 0x451c, Data4: [8]byte{0xab, 0x95, 0xac, 0x03, 0x4b, 0x8e, 0x17, 0x31}}
	MF_MT_AUDIO_BLOCK_ALIGNMENT       = windows.GUID{Data1: 0x322de230, Data2: 0x9eeb, Data3: 0x43bd, Data4: [8]byte{0xab, 0x7a, 0xff, 0x41, 0x22, 0x51, 0x54, 0x1d}}
	MF_MT_AUDIO_BITS_PER_SAMPLE       = windows.GUID{Data1: 0xf2deb57f, Data2: 0x40fa, Data3: 0x4764, Data4: [8]byte{0xaa, 0x33, 0xed, 0x4f, 0x2d, 0x1f, 0xf6, 0x69}}
	MF_MT_AUDIO_VALID_BITS_PER_SAMPLE = windows.GUID{Data1: 0xd9bf8d6a, Data2: 0x9530, Data3: 0x4b7c, Data4: [8]byte{0x9d, 0xdf, 0xff, 0x6f, 0xd5, 0x8b, 0xbd, 0x06}}
	MF_MT_AUDIO_CHANNEL_MASK          = windows.GUID{Data1: 0x55fb5765, Data2: 0x644a, Data3: 0x4caf, Data4: [8]byte{0x84, 0x79, 0x93, 0x89, 0x83, 0xbb, 0x15, 0x88}}

	MF_MT_ALL_SAMPLES_INDEPENDENT = windows.GUID{Data1: 0xc9173739, Data2: 0x5e56, Data3: 0x461c, Data4: [8]byte{0xb7, 0x13, 0x46, 0xfb, 0x99, 0x5c, 0xb9, 0x5f}}

	MFAudioFormat_Base = windows.GUID{Data1: 0x00000000, Data2: 0x0000, Data3: 0x0010, Data4: [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	MFMediaType_Audio  = windows.GUID{Data1: 0x73647561, Data2: 0x0000, Data3: 0x0010, Data4: [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}

	MF_METADATA_PROVIDER_SERVICE = windows.GUID{Data1: 0xdb214084, Data2: 0x58a4, Data3: 0x4d2e, Data4: [8]byte{0xb8, 0x4f, 0x6f, 0x75, 0x5b, 0x2f, 0x7a, 0xd}}
	MF_PROPERTY_HANDLER_SERVICE  = windows.GUID{Data1: 0xa3face02, Data2: 0x32b8, Data3: 0x41dd, Data4: [8]byte{0x90, 0xe7, 0x5f, 0xef, 0x7c, 0x89, 0x91, 0xb5}}
	MF_MEDIASOURCE_SERVICE       = windows.GUID{Data1: 0xf09992f7, Data2: 0x9fba, Data3: 0x4c4a, Data4: [8]byte{0xa3, 0x7f, 0x8c, 0x47, 0xb4, 0xe1, 0xdf, 0xe7}}

	IID_IMFMetadataProvider = windows.GUID{Data1: 0x56181D2D, Data2: 0xE221, Data3: 0x4adb, Data4: [8]byte{0xB1, 0xC8, 0x3C, 0xEE, 0x6A, 0x53, 0xF7, 0x6F}}
	IID_IPropertyStore      = windows.GUID{Data1: 0x886d8eeb, Data2: 0x8cf2, Data3: 0x4446, Data4: [8]byte{0x8d, 0x02, 0xcd, 0xba, 0x1d, 0xbd, 0xcf, 0x99}}
	IID_IMFMediaSource      = windows.GUID{0x279A808D, 0xAEC7, 0x40C8, [8]byte{0x9C, 0x6B, 0xA6, 0xB4, 0x92, 0xC7, 0x8A, 0x66}}

	PKEY_Title            = PropertyKey{windows.GUID{0xF29F85E0, 0x4FF9, 0x1068, [8]byte{0xAB, 0x91, 0x08, 0x00, 0x2B, 0x27, 0xB3, 0xD9}}, 2}
	PKEY_Music_Artist     = PropertyKey{windows.GUID{0x56A3372E, 0xCE9C, 0x11D2, [8]byte{0x9F, 0x0E, 0x00, 0x60, 0x97, 0xC6, 0x86, 0xF6}}, 2}
	PKEY_Music_AlbumTitle = PropertyKey{windows.GUID{0x56A3372E, 0xCE9C, 0x11D2, [8]byte{0x9F, 0x0E, 0x00, 0x60, 0x97, 0xC6, 0x86, 0xF6}}, 4}
	PKEY_Media_Duration   = PropertyKey{windows.GUID{0x64440490, 0x4C8B, 0x11D1, [8]byte{0x8B, 0x70, 0x08, 0x00, 0x36, 0xB1, 0x1A, 0x03}}, 3}
)

const (
	MF_SDK_VERSION = 0x0002
	MF_VERSION_API = 0x0070
	MF_VERSION     = (MF_SDK_VERSION << 16) | MF_VERSION_API

	MF_SOURCE_READER_FIRST_AUDIO_STREAM = 0xFFFFFFFD
	MF_SOURCE_READER_ANY_STREAM         = 0xFFFFFFFE
	MF_SOURCE_READER_MEDIASOURCE        = 0xFFFFFFFF

	MFSTARTUP_FULL = uint32(0)

	VT_BOOL   = 11
	VT_UI4    = 19
	VT_UI8    = 21
	VT_LPWSTR = 31
)

func StartMediaFoundation() (err error) {
	r1, _, _ := MFStartup.Call(uintptr(MF_VERSION), uintptr(MFSTARTUP_FULL))
	if uint32(r1) != uint32(windows.S_OK) {
		return errors.New("could not start media foundation")
	}

	return nil
}

func CreateSourceReaderFromFile(path string) (reader *MFSourceReader, err error) {
	var ReaderPtr **MFSourceReaderVtbl

	path += "\x00" // null terminate

	encoded := utf16.Encode([]rune(path))
	r1, _, _ := MFCreateSourceReaderFromURL.Call(uintptr(unsafe.Pointer(&encoded[0])), 0, uintptr(unsafe.Pointer(&ReaderPtr)))
	if uint32(r1) != uint32(windows.S_OK) {
		return nil, errors.New("could not create reader")
	}

	return &MFSourceReader{ptr: uintptr(unsafe.Pointer(ReaderPtr)), vtbl: *ReaderPtr}, nil

}

func GetSARSinkWriter() (writer *MFSinkWriter, err error) {
	var sinkPtr *uintptr
	//get SAR media sink
	r1, _, err := MFCreateAudioRenderer.Call(0, uintptr(unsafe.Pointer(&sinkPtr)))
	if uint32(r1) != uint32(windows.S_OK) {
		return nil, errors.New("could not create audio renderer")
	}

	//get writer from sink
	var writerPtr **MFSinkWriterVtbl
	r1, _, err = MFCreateSinkWriterFromMediaSink.Call(uintptr(unsafe.Pointer(sinkPtr)), 0, uintptr(unsafe.Pointer(&writerPtr)))
	if uint32(r1) != uint32(windows.S_OK) {
		return nil, errors.New("could not create sink writer")
	}

	return &MFSinkWriter{ptr: uintptr(unsafe.Pointer(writerPtr)), vtbl: *writerPtr}, nil
}

func getMediaTypeFromWaveFormat(w *WaveFormatExtensible) (mediaType *MFMediaType, err error) {
	var mediaTypePtr **MFMediaTypeVtbl
	r1, _, _ := MFCreateAudioMediaType.Call(uintptr(unsafe.Pointer(w)), uintptr(unsafe.Pointer(&mediaTypePtr)))
	if uint32(r1) != uint32(windows.S_OK) {
		return nil, errors.New("could not convert wave format to media type")
	}

	return &MFMediaType{ptr: uintptr(unsafe.Pointer(mediaTypePtr)), vtbl: *mediaTypePtr}, nil
}

func getWaveFormatFromMediaType(m *MFMediaType) (waveFormat *WaveFormatExtensible, err error) {
	var size *uint32
	r1, _, _ := MFCreateWaveFormatExFromMFMediaType.Call(m.ptr, uintptr(unsafe.Pointer(&waveFormat)), uintptr(unsafe.Pointer(&size)), 1)
	if uint32(r1) != uint32(windows.S_OK) {
		return nil, errors.New("could not convert media type to wave format")
	}

	return waveFormat, nil
}

type MFSourceReader struct {
	ptr  uintptr
	vtbl *MFSourceReaderVtbl
}

type MFSourceReaderVtbl struct {
	queryInterface uintptr
	addref         uintptr
	release        uintptr

	getStreamSelection  uintptr
	SetStreamSelection  uintptr
	GetNativeMediaType  uintptr
	GetCurrentMediaType uintptr
	SetCurrentMediaType uintptr
	SetCurrentPosition  uintptr
	ReadSample          uintptr
	Flush               uintptr
	GetServiceForStream uintptr
}

func (s MFSourceReader) ReadSample(streamIndex uint32, controlFlags uint32) (actualStreamIndex uint32, streamFlags uint32, timeStamp int64, sample *MFSample, err error) {
	var samplePtr **MFSampleVtbl
	r1, _, _ := syscall.SyscallN(s.vtbl.ReadSample, s.ptr, uintptr(streamIndex), uintptr(controlFlags), uintptr(unsafe.Pointer(&actualStreamIndex)), uintptr(unsafe.Pointer(&streamFlags)), uintptr(unsafe.Pointer(&timeStamp)), uintptr(unsafe.Pointer(&samplePtr)))
	if uint32(r1) != uint32(windows.S_OK) {
		return 0, 0, 0, nil, errors.New("could not read sample")
	} else if streamFlags&0x2 > 0 {
		return 0, 0, 0, nil, io.EOF
	}

	return actualStreamIndex, streamFlags, timeStamp, &MFSample{ptr: uintptr(unsafe.Pointer(samplePtr)), vtbl: *samplePtr}, nil
}

func (s MFSourceReader) SetWaveFormat(w *WaveFormatExtensible) (err error) {

	mediaType, err := getMediaTypeFromWaveFormat(w)
	if err != nil {
		return err
	}
	r1, _, _ := syscall.SyscallN(s.vtbl.SetCurrentMediaType, s.ptr, uintptr(0xFFFFFFFD), uintptr(0), uintptr(unsafe.Pointer(mediaType.ptr)))
	if uint32(r1) != uint32(windows.S_OK) {
		return errors.New("could not set wave format")
	}

	return nil
}

func (s MFSourceReader) GetWaveFormat() (w *WaveFormatExtensible, err error) {
	var mediaTypePtr **MFMediaTypeVtbl
	r1, _, _ := syscall.SyscallN(s.vtbl.GetCurrentMediaType, s.ptr, uintptr(MF_SOURCE_READER_FIRST_AUDIO_STREAM), uintptr(unsafe.Pointer(&mediaTypePtr)))
	if uint32(r1) != uint32(windows.S_OK) {
		return nil, errors.New("could not get wave format")
	}

	mediaType := &MFMediaType{ptr: uintptr(unsafe.Pointer(mediaTypePtr)), vtbl: *mediaTypePtr}

	return getWaveFormatFromMediaType(mediaType)

}

func (s MFSourceReader) GetMediaSource() (source *MFMediaSource, err error) {
	var mediaSourcePtr **MFMediaSourceVtbl
	r1, _, _ := syscall.SyscallN(s.vtbl.GetServiceForStream, s.ptr, uintptr(MF_SOURCE_READER_MEDIASOURCE), uintptr(unsafe.Pointer(&GUID_null)), uintptr(unsafe.Pointer(&IID_IMFMediaSource)), uintptr(unsafe.Pointer(&mediaSourcePtr)))
	if uint32(r1) != uint32(windows.S_OK) {
		return nil, errors.New("could not get media source")
	}

	source = &MFMediaSource{ptr: uintptr(unsafe.Pointer(mediaSourcePtr)), vtbl: *mediaSourcePtr}
	return source, nil
}

type MFSample struct {
	ptr  uintptr
	vtbl *MFSampleVtbl
}

type MFSampleVtbl struct {
	queryInterface uintptr
	addref         uintptr
	release        uintptr

	GetItem            uintptr
	GetItemType        uintptr
	CompareItem        uintptr
	Compare            uintptr
	GetUINT32          uintptr
	GetUINT64          uintptr
	GetDouble          uintptr
	GetGUID            uintptr
	GetStringLength    uintptr
	GetString          uintptr
	GetAllocatedString uintptr
	GetBlobSize        uintptr
	GetBlob            uintptr
	GetAllocatedBlob   uintptr
	GetUnknown         uintptr
	SetItem            uintptr
	DeleteItem         uintptr
	DeleteAllItems     uintptr
	SetUINT32          uintptr
	SetUINT64          uintptr
	SetDouble          uintptr
	SetGUID            uintptr
	SetString          uintptr
	SetBlob            uintptr
	SetUnknown         uintptr
	LockStore          uintptr
	UnlockStore        uintptr
	GetCount           uintptr
	GetItemByIndex     uintptr
	CopyAllItems       uintptr

	GetSampleFlags            uintptr
	SetSampleFlags            uintptr
	GetSampleTime             uintptr
	SetSampleTime             uintptr
	GetSampleDuration         uintptr
	SetSampleDuration         uintptr
	GetBufferCount            uintptr
	GetBufferByIndex          uintptr
	ConvertToContiguousBuffer uintptr
	AddBuffer                 uintptr
	RemoveBufferByIndex       uintptr
	RemoveAllBuffers          uintptr
	GetTotalLength            uintptr
}

func (s MFSample) ConvertToContiguousBuffer() (mediaBuffer *MFMediaBuffer, err error) {
	var mediaBufferPtr **MFMediaBufferVtbl
	r1, _, _ := syscall.SyscallN(s.vtbl.ConvertToContiguousBuffer, s.ptr, uintptr(unsafe.Pointer(&mediaBufferPtr)))
	if uint32(r1) != uint32(windows.S_OK) {
		return nil, errors.New("could not convert to contiguous buffer")
	}

	return &MFMediaBuffer{ptr: uintptr(unsafe.Pointer(mediaBufferPtr)), vtbl: *mediaBufferPtr}, nil
}

type MFMediaBuffer struct {
	ptr  uintptr
	vtbl *MFMediaBufferVtbl
}

type MFMediaBufferVtbl struct {
	queryInterface uintptr
	addref         uintptr
	release        uintptr

	Lock             uintptr
	Unlock           uintptr
	GetCurrentLength uintptr
	SetCurrentLength uintptr
	GetMaxLength     uintptr
}

func (b MFMediaBuffer) Lock() (bufferPtr *byte, maxLength uint32, curLength uint32, err error) {
	r1, _, _ := syscall.SyscallN(b.vtbl.Lock, b.ptr, uintptr(unsafe.Pointer(&bufferPtr)), uintptr(unsafe.Pointer(&maxLength)), uintptr(unsafe.Pointer(&curLength)))
	if uint32(r1) != uint32(windows.S_OK) {
		return nil, 0, 0, errors.New("could not lock buffer")
	}

	return bufferPtr, maxLength, curLength, nil
}

func (b MFMediaBuffer) Unlock() (err error) {
	r1, _, _ := syscall.SyscallN(b.vtbl.Unlock, b.ptr)
	if uint32(r1) != uint32(windows.S_OK) {
		return errors.New("could not unlock buffer")
	}

	return nil
}

type MFMediaType struct {
	ptr  uintptr
	vtbl *MFMediaTypeVtbl
}

type MFMediaTypeVtbl struct {
	queryInterface uintptr
	addref         uintptr
	release        uintptr

	GetItem            uintptr
	GetItemType        uintptr
	CompareItem        uintptr
	Compare            uintptr
	GetUINT32          uintptr
	GetUINT64          uintptr
	GetDouble          uintptr
	GetGUID            uintptr
	GetStringLength    uintptr
	GetString          uintptr
	GetAllocatedString uintptr
	GetBlobSize        uintptr
	GetBlob            uintptr
	GetAllocatedBlob   uintptr
	GetUnknown         uintptr
	SetItem            uintptr
	DeleteItem         uintptr
	DeleteAllItems     uintptr
	SetUINT32          uintptr
	SetUINT64          uintptr
	SetDouble          uintptr
	SetGUID            uintptr
	SetString          uintptr
	SetBlob            uintptr
	SetUnknown         uintptr
	LockStore          uintptr
	UnlockStore        uintptr
	GetCount           uintptr
	GetItemByIndex     uintptr
	CopyAllItems       uintptr

	GetMajorType       uintptr
	IsCompressedFormat uintptr
	IsEqual            uintptr
	GetRepresentation  uintptr
	FreeRepresentation uintptr
}

func (m MFMediaType) SetGUID(guidKey windows.GUID, guidValue windows.GUID) (err error) {
	r1, _, _ := syscall.SyscallN(m.vtbl.SetGUID, m.ptr, uintptr(unsafe.Pointer(&guidKey)), uintptr(unsafe.Pointer(&guidValue)))

	if uint32(r1) != uint32(windows.S_OK) {
		return errors.New("could not set GUID")
	}

	return nil
}

func (m MFMediaType) SetUINT32(guidKey windows.GUID, uint32Value uint32) (err error) {
	r1, _, _ := syscall.SyscallN(m.vtbl.SetUINT32, m.ptr, uintptr(unsafe.Pointer(&guidKey)), uintptr(uint32Value))

	if uint32(r1) != uint32(windows.S_OK) {
		return errors.New("could not set GUID")
	}

	return nil
}

type MFSinkWriter struct {
	ptr  uintptr
	vtbl *MFSinkWriterVtbl
}

type MFSinkWriterVtbl struct {
	queryInterface uintptr
	addref         uintptr
	release        uintptr

	AddStream           uintptr
	SetInputMediaType   uintptr
	BeginWriting        uintptr
	WriteSample         uintptr
	SendStreamTick      uintptr
	PlaceMarker         uintptr
	NotifyEndOfSegment  uintptr
	Flush               uintptr
	Finalize            uintptr
	GetServiceForStream uintptr
	GetStatistics       uintptr
}

func (w MFSinkWriter) BeginWriting() (err error) {
	r1, _, _ := syscall.SyscallN(w.vtbl.BeginWriting, w.ptr)

	if uint32(r1) != uint32(windows.S_OK) {
		return errors.New("could not begin writing")
	}

	return nil
}

type MFMediaSource struct {
	ptr  uintptr
	vtbl *MFMediaSourceVtbl
}

type MFMediaSourceVtbl struct {
	queryInterface uintptr
	addref         uintptr
	release        uintptr

	GetEvent      uintptr
	BeginGetEvent uintptr
	EndGetEvent   uintptr
	QueueEvent    uintptr

	GetCharacteristics           uintptr
	CreatePresentationDescriptor uintptr
	Start                        uintptr
	Stop                         uintptr
	Pause                        uintptr
	Shutdown                     uintptr
}

func (source MFMediaSource) GetPropertyStore() (propStore *PropertyStore, err error) {
	var propStorePtr **PropertyStoreVtbl
	r1, _, _ := MFGetService.Call(source.ptr, uintptr(unsafe.Pointer(&MF_PROPERTY_HANDLER_SERVICE)), uintptr(unsafe.Pointer(&IID_IPropertyStore)), uintptr(unsafe.Pointer(&propStorePtr)))
	if uint32(r1) != uint32(windows.S_OK) {
		return nil, errors.New("could not get property store")
	}

	return &PropertyStore{ptr: uintptr(unsafe.Pointer(propStorePtr)), vtbl: *propStorePtr}, nil
}

type PropertyStore struct {
	ptr  uintptr
	vtbl *PropertyStoreVtbl
}

type PropertyStoreVtbl struct {
	queryInterface uintptr
	addref         uintptr
	release        uintptr

	GetCount uintptr
	GetAt    uintptr
	GetValue uintptr
	SetValue uintptr
	Commit   uintptr
}

func (p PropertyStore) GetCount() (count uint32, err error) {
	r1, _, _ := syscall.SyscallN(p.vtbl.GetCount, p.ptr, uintptr(unsafe.Pointer(&count)))
	if uint32(r1) != uint32(windows.S_OK) {
		return 0, errors.New("could not get count")
	}
	return count, nil
}

func (p PropertyStore) GetAt(prop uint32) (propKey PropertyKey, err error) {
	r1, _, _ := syscall.SyscallN(p.vtbl.GetAt, p.ptr, uintptr(prop), uintptr(unsafe.Pointer(&propKey)))
	if uint32(r1) != uint32(windows.S_OK) {
		return PropertyKey{}, fmt.Errorf("could not get property key: %X", r1)
	}

	return propKey, nil
}

func (p PropertyStore) GetValue(key *PropertyKey) (value PropVariant, err error) {
	r1, _, _ := syscall.SyscallN(p.vtbl.GetValue, p.ptr, uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(&value)))
	if uint32(r1) != uint32(windows.S_OK) {
		return PropVariant{}, errors.New("could not get property value")
	}
	return value, nil
}

type PropertyKey struct {
	Fmtid windows.GUID
	Pid   uint32
}

type PropVariant struct {
	PropType  uint16
	reserved1 uint16
	reserved2 uint16
	reserved3 uint16
	Data      uint64
}
