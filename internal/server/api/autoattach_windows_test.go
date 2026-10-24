//go:build windows

package api

import (
	"testing"
	"unsafe"
)

func TestUSBIP981Layout(t *testing.T) {
	var request attachIOCTL_981
	if unsafe.Sizeof(request) != 1124 || unsafe.Offsetof(request.Tail) != 1104 {
		t.Fatalf("incorrect USBIP 0.9.8.1 request layout: size=%d tail=%d", unsafe.Sizeof(request), unsafe.Offsetof(request.Tail))
	}
	if unsafe.Offsetof(request.importedDeviceLocation) != 4 || unsafe.Offsetof(request.deviceLocation) != 12 {
		t.Fatal("incorrect imported device offsets")
	}
	if ioctlPluginHardware != 0x22e018 {
		t.Fatalf("expected one-shot import IOCTL, got %#x", ioctlPluginHardware)
	}
}
