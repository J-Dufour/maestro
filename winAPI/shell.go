package winAPI

import (
	"errors"
	"fmt"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	Shell32 = windows.NewLazyDLL("Shell32.dll")

	SHGetPropertyStoreFromParsingName = Shell32.NewProc("SHGetPropertyStoreFromParsingName")

	IID_IPropertyStore = windows.GUID{Data1: 0x886d8eeb, Data2: 0x8cf2, Data3: 0x4446, Data4: [8]byte{0x8d, 0x02, 0xcd, 0xba, 0x1d, 0xbd, 0xcf, 0x99}}

	PKEY_Title               = PropertyKey{windows.GUID{0xF29F85E0, 0x4FF9, 0x1068, [8]byte{0xAB, 0x91, 0x08, 0x00, 0x2B, 0x27, 0xB3, 0xD9}}, 2}
	PKEY_Music_DisplayArtist = PropertyKey{windows.GUID{0xFD122953, 0xFA93, 0x4EF7, [8]byte{0x92, 0xC3, 0x04, 0xC9, 0x46, 0xB2, 0xF7, 0xC8}}, 100}
	PKEY_Music_AlbumTitle    = PropertyKey{windows.GUID{0x56A3372E, 0xCE9C, 0x11D2, [8]byte{0x9F, 0x0E, 0x00, 0x60, 0x97, 0xC6, 0x86, 0xF6}}, 4}
	PKEY_Media_Duration      = PropertyKey{windows.GUID{0x64440490, 0x4C8B, 0x11D1, [8]byte{0x8B, 0x70, 0x08, 0x00, 0x36, 0xB1, 0x1A, 0x03}}, 3}
)

const (
	VT_BOOL   = 11
	VT_UI4    = 19
	VT_I8     = 20
	VT_UI8    = 21
	VT_LPWSTR = 31
)

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

func GetPropertyStoreFromParsingName(name string) (pStore *PropertyStore, err error) {
	var propStorePtr **PropertyStoreVtbl

	name += "\x00" // null terminate

	encoded := utf16.Encode([]rune(name))

	r1, _, _ := SHGetPropertyStoreFromParsingName.Call(uintptr(unsafe.Pointer(&encoded[0])), 0, 0, uintptr(unsafe.Pointer(&IID_IPropertyStore)), uintptr(unsafe.Pointer(&propStorePtr)))
	if uint32(r1) != uint32(windows.S_OK) {
		return nil, errors.New("could not create propstore")
	}

	return &PropertyStore{ptr: uintptr(unsafe.Pointer(propStorePtr)), vtbl: *propStorePtr}, nil

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
