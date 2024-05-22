package winAPI

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	GUID_null              = windows.GUID{Data1: 0x00000000, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}}
	MMDeviceEnumGUID       = windows.GUID{Data1: 0xBCDE0395, Data2: 0xE52F, Data3: 0x467C, Data4: [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	MMDeviceEnumRefID      = windows.GUID{Data1: 0xA95664D2, Data2: 0x9614, Data3: 0x4F35, Data4: [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	AudioClientRefID       = windows.GUID{Data1: 0x1CB9AD4C, Data2: 0xDBFA, Data3: 0x4c32, Data4: [8]byte{0xB1, 0x78, 0xC2, 0xF5, 0x68, 0xA7, 0x03, 0xB2}}
	AudioRenderClientRefID = windows.GUID{Data1: 0xF294ACFC, Data2: 0x3146, Data3: 0x4483, Data4: [8]byte{0xA7, 0xBF, 0xAD, 0xDC, 0xA7, 0xC2, 0x60, 0xE2}}
)
var (
	ole32        = windows.NewLazyDLL("ole32.dll")
	coCreateInst = ole32.NewProc("CoCreateInstance")
)

type AudioRenderClient struct {
	ptr  uintptr
	vtbl *AudioRenderClientVtbl
}

type AudioRenderClientVtbl struct {
	queryInterface uintptr
	addref         uintptr
	release        uintptr

	getBuffer     uintptr
	releaseBuffer uintptr
}

type AudioClient struct {
	ptr  uintptr
	vtbl *AudioClientVtbl
}

type AudioClientVtbl struct {
	queryInterface uintptr
	addref         uintptr
	release        uintptr

	initialize        uintptr
	getBufferSize     uintptr
	getStreamLatency  uintptr
	getCurrentPadding uintptr
	isFormatSupported uintptr
	getMixFormat      uintptr
	getDevicePeriod   uintptr
	start             uintptr
	stop              uintptr
	reset             uintptr
	setEventHandle    uintptr
	getService        uintptr
}

type MMDeviceVtbl struct {
	queryInterface uintptr
	addref         uintptr
	release        uintptr

	activate          uintptr
	openPropertyStore uintptr
	getId             uintptr
	getState          uintptr
}

type MMDeviceEnumVtbl struct {
	queryInterface uintptr
	addref         uintptr
	release        uintptr

	enumAudioEndpoints                     uintptr
	getDefaultAudioEndpoint                uintptr
	getDevice                              uintptr
	registerEndpointNotificationCallback   uintptr
	unregisterEndpointNotificationCallback uintptr
}

type WaveFormatExtensible struct {
	WFormatTag      uint16
	NChannels       uint16
	NSamplesPerSec  uint32
	NAvgBytesPerSec uint32
	NBlockAlign     uint16
	WBitsPerSample  uint16
	CbSize          uint16

	Reserved      uint16
	DwChannelMask uint32
	SubFormat     windows.GUID
}

func (a AudioClient) Initialize(sharemode int32, flags int32, refTime int64, period int64, waveFormat *WaveFormatExtensible) (err error) {
	r1, _, err := syscall.SyscallN(a.vtbl.initialize, a.ptr, uintptr(sharemode), uintptr(flags), uintptr(refTime), uintptr(period), uintptr(unsafe.Pointer(waveFormat)), 0)
	runtime.KeepAlive(waveFormat)
	runtime.KeepAlive(a.ptr)
	if uint32(r1) != uint32(windows.S_OK) {
		err = errors.New("cannot initialize")
		return err
	}
	return nil
}

func (a AudioClient) IsFormatSupported(sharemode int32, format *WaveFormatExtensible) (formatSupported bool, closestMatch *WaveFormatExtensible, err error) {
	var closest *WaveFormatExtensible
	r1, _, err := syscall.SyscallN(a.vtbl.isFormatSupported, a.ptr, uintptr(sharemode), uintptr(unsafe.Pointer(format)), uintptr(unsafe.Pointer(&closest)))
	if uint32(r1) == uint32(windows.S_FALSE) {
		return false, closest, nil
	} else if uint32(r1) != uint32(windows.S_OK) {
		err = errors.New("format unsupported")
		return false, nil, err
	}
	return true, format, nil
}

func (a AudioClient) GetMixFormat() (format *WaveFormatExtensible, err error) {
	r1, _, err := syscall.SyscallN(a.vtbl.getMixFormat, a.ptr, uintptr(unsafe.Pointer(&format)))
	if uint32(r1) != uint32(windows.S_OK) {
		err = errors.New("cannot get mix format")
		return nil, err
	}

	return format, nil
}

func (a AudioClient) GetRenderClient() (renderClient *AudioRenderClient, err error) {
	var renderClientVtbl **AudioRenderClientVtbl

	r1, _, err := syscall.SyscallN(a.vtbl.getService, a.ptr, uintptr(unsafe.Pointer(&AudioRenderClientRefID)), uintptr(unsafe.Pointer(&renderClientVtbl)))
	if uint32(r1) != uint32(windows.S_OK) {
		err = errors.New("could not get render client")
		return nil, err
	}

	return &AudioRenderClient{ptr: uintptr(unsafe.Pointer(renderClientVtbl)), vtbl: *renderClientVtbl}, nil
}

func (a AudioClient) GetCurrentPadding() (padding uint32, err error) {
	r1, _, err := syscall.SyscallN(a.vtbl.getCurrentPadding, a.ptr, uintptr(unsafe.Pointer(&padding)))
	if uint32(r1) != uint32(windows.S_OK) {
		err = errors.New("could not get padding")
		return 0, err
	}
	return padding, nil
}

func (a AudioClient) GetBufferSize() (size uint32, err error) {
	r1, _, err := syscall.SyscallN(a.vtbl.getBufferSize, a.ptr, uintptr(unsafe.Pointer(&size)))
	if uint32(r1) != uint32(windows.S_OK) {
		err = errors.New("could not get buffer size")
		return 0, err
	}
	return size, nil
}

func (a AudioClient) Start() (err error) {
	r1, _, err := syscall.SyscallN(a.vtbl.start, a.ptr)
	if uint32(r1) != uint32(windows.S_OK) {
		err = errors.New("could not start")
		return err
	}
	return nil
}

func (a AudioClient) Stop() (err error) {
	r1, _, err := syscall.SyscallN(a.vtbl.stop, a.ptr)
	if uint32(r1) != uint32(windows.S_OK) {
		err = errors.New("could not stop")
		return err
	}
	return nil
}

func (a AudioClient) Reset() (err error) {
	r1, _, err := syscall.SyscallN(a.vtbl.reset, a.ptr)
	if uint32(r1) != uint32(windows.S_OK) {
		err = errors.New("could not reset")
		return err
	}
	return nil
}

func (a AudioRenderClient) GetBuffer(frames uint32) (buffStart *byte, err error) {
	r1, _, err := syscall.SyscallN(a.vtbl.getBuffer, a.ptr, uintptr(frames), uintptr(unsafe.Pointer(&buffStart)))
	if uint32(r1) != uint32(windows.S_OK) {
		err = errors.New("could not get buffer")
		return nil, err
	}

	return buffStart, nil
}

func (a AudioRenderClient) ReleaseBuffer(frames uint32) (err error) {
	r1, _, err := syscall.SyscallN(a.vtbl.releaseBuffer, a.ptr, uintptr(frames))
	if uint32(r1) != uint32(windows.S_OK) {
		err = errors.New("could not get buffer")
		return err
	}

	return nil
}

func GetDefaultClient() (client *AudioClient, err error) {
	var (
		MMDeviceEnumPtr **MMDeviceEnumVtbl
		MMDevicePtr     **MMDeviceVtbl
		clientPtr       **AudioClientVtbl
	)

	// get MMDeviceEnum
	r1, _, _ := coCreateInst.Call(uintptr(unsafe.Pointer(&MMDeviceEnumGUID)), 0, windows.CLSCTX_INPROC_SERVER, uintptr(unsafe.Pointer(&MMDeviceEnumRefID)), uintptr(unsafe.Pointer(&MMDeviceEnumPtr)))
	if uint32(r1) != uint32(windows.S_OK) {
		err = errors.New("cannot create device enumerator")
		return nil, err
	}
	deviceEnum := *MMDeviceEnumPtr

	// get default MMDevice
	r1, _, _ = syscall.SyscallN(deviceEnum.getDefaultAudioEndpoint, uintptr(unsafe.Pointer(MMDeviceEnumPtr)), 0, 1, uintptr(unsafe.Pointer(&MMDevicePtr)))
	if uint32(r1) != uint32(windows.S_OK) {
		err = errors.New("cannot find default audio endpoint")
		return nil, err
	}
	device := *MMDevicePtr

	// get AudioClient
	r1, _, _ = syscall.SyscallN(device.activate, uintptr(unsafe.Pointer(MMDevicePtr)), uintptr(unsafe.Pointer(&AudioClientRefID)), 0x000000017, 0, uintptr(unsafe.Pointer(&clientPtr)))
	if uint32(r1) != uint32(windows.S_OK) {
		fmt.Println(r1)
		err = errors.New("cannot instantiate audio client")
		return nil, err
	}

	//clientVtbl := *clientPtr
	client = &AudioClient{ptr: uintptr(unsafe.Pointer(clientPtr)), vtbl: *clientPtr}

	return client, nil
}
