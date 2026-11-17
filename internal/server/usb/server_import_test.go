package usb

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	usbdevice "github.com/Alia5/VIIPER/usb"
	"github.com/Alia5/VIIPER/usbip"
	"github.com/Alia5/VIIPER/virtualbus"
)

type importTestDevice struct{ id int }

func (*importTestDevice) HandleTransfer(context.Context, uint32, uint32, []byte) []byte {
	return nil
}
func (*importTestDevice) GetDescriptor() *usbdevice.Descriptor  { return &usbdevice.Descriptor{} }
func (*importTestDevice) GetDeviceSpecificArgs() map[string]any { return nil }

func importServer(t *testing.T) (*Server, *virtualbus.VirtualBus, *importTestDevice) {
	t.Helper()
	dev := &importTestDevice{id: 1}
	bus := virtualbus.New(1)
	t.Cleanup(func() { _ = bus.Close() })
	if _, err := bus.Add(dev); err != nil {
		t.Fatal(err)
	}
	srv := New(ServerConfig{ConnectionTimeout: 5 * time.Second}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err := srv.AddBus(bus); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv, bus, dev
}

func importPipe(t *testing.T, srv *Server) (net.Conn, <-chan struct{}) {
	t.Helper()
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.handleConn(server)
	}()
	t.Cleanup(func() {
		_ = client.Close()
		waitImportClosed(t, done)
	})
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	return client, done
}

func waitImportClosed(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("import connection did not close")
	}
}

func requestImport(conn net.Conn, busID string) (uint32, error) {
	request := make([]byte, 40)
	binary.BigEndian.PutUint16(request[0:2], usbip.Version)
	binary.BigEndian.PutUint16(request[2:4], usbip.OpReqImport)
	copy(request[8:], busID)
	if _, err := conn.Write(request); err != nil {
		return 0, err
	}
	var header [8]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return 0, err
	}
	if binary.BigEndian.Uint16(header[2:4]) != usbip.OpRepImport {
		return 0, fmt.Errorf("unexpected import reply")
	}
	status := binary.BigEndian.Uint32(header[4:8])
	if status == 0 {
		if _, err := io.CopyN(io.Discard, conn, 312); err != nil {
			return 0, err
		}
	}
	return status, nil
}

func assertImport(t *testing.T, conn net.Conn, busID string, want uint32) {
	t.Helper()
	status, err := requestImport(conn, busID)
	if err != nil || status != want {
		t.Fatalf("import %s: status=%d, error=%v, want status=%d", busID, status, err, want)
	}
}

func TestImportOwnership(t *testing.T) {
	srv, _, _ := importServer(t)
	first, done := importPipe(t, srv)
	assertImport(t, first, "1-1", 0)
	second, _ := importPipe(t, srv)
	assertImport(t, second, "1-1", 2)
	_ = first.Close()
	waitImportClosed(t, done)
	third, _ := importPipe(t, srv)
	assertImport(t, third, "1-1", 0)
}

func TestImportOwnershipConcurrent(t *testing.T) {
	srv, _, _ := importServer(t)
	const count = 16
	results := make(chan uint32, count)
	errors := make(chan error, count)
	for i := 0; i < count; i++ {
		conn, _ := importPipe(t, srv)
		go func() {
			status, err := requestImport(conn, "1-1")
			results <- status
			errors <- err
		}()
	}
	accepted := 0
	for i := 0; i < count; i++ {
		status := <-results
		if status == 0 {
			accepted++
		} else if status != 2 {
			t.Fatalf("unexpected status %d", status)
		}
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d concurrent imports, want exactly one", accepted)
	}
}

func TestImportOwnershipFailedReply(t *testing.T) {
	srv, _, _ := importServer(t)
	conn, done := importPipe(t, srv)
	request := make([]byte, 40)
	binary.BigEndian.PutUint16(request[0:2], usbip.Version)
	binary.BigEndian.PutUint16(request[2:4], usbip.OpReqImport)
	copy(request[8:], "1-1")
	if _, err := conn.Write(request); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	waitImportClosed(t, done)
	next, _ := importPipe(t, srv)
	assertImport(t, next, "1-1", 0)
}

func TestImportOwnershipRemoval(t *testing.T) {
	for _, removeBus := range []bool{false, true} {
		t.Run(fmt.Sprintf("bus=%t", removeBus), func(t *testing.T) {
			srv, bus, dev := importServer(t)
			first, done := importPipe(t, srv)
			assertImport(t, first, "1-1", 0)
			if removeBus {
				if err := srv.RemoveBus(1); err != nil {
					t.Fatal(err)
				}
				bus = virtualbus.New(1)
				t.Cleanup(func() { _ = bus.Close() })
				if err := srv.AddBus(bus); err != nil {
					t.Fatal(err)
				}
			} else if err := bus.Remove(dev); err != nil {
				t.Fatal(err)
			}
			if _, err := bus.Add(&importTestDevice{id: 2}); err != nil {
				t.Fatal(err)
			}
			next, _ := importPipe(t, srv)
			assertImport(t, next, "1-1", 0)
			waitImportClosed(t, done)
			duplicate, _ := importPipe(t, srv)
			assertImport(t, duplicate, "1-1", 2)
		})
	}
}

func TestImportOwnershipShutdown(t *testing.T) {
	srv, _, _ := importServer(t)
	first, done := importPipe(t, srv)
	assertImport(t, first, "1-1", 0)
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	waitImportClosed(t, done)
	next, _ := importPipe(t, srv)
	assertImport(t, next, "1-1", 4)
}

func TestImportOwnershipIndependentDevices(t *testing.T) {
	srv, bus, _ := importServer(t)
	if _, err := bus.Add(&importTestDevice{id: 2}); err != nil {
		t.Fatal(err)
	}
	first, _ := importPipe(t, srv)
	assertImport(t, first, "1-1", 0)
	second, _ := importPipe(t, srv)
	assertImport(t, second, "1-2", 0)
	missing, _ := importPipe(t, srv)
	assertImport(t, missing, "1-3", 4)
}

func TestImportOwnershipMalformedBusID(t *testing.T) {
	srv, _, _ := importServer(t)
	conn, done := importPipe(t, srv)
	if _, err := requestImport(conn, "12345678901234567890123456789012"); err == nil {
		t.Fatal("unterminated bus ID was accepted")
	}
	waitImportClosed(t, done)
	next, _ := importPipe(t, srv)
	assertImport(t, next, "1-1", 0)
}
