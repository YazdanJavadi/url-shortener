package server

import (
	"net"
	"testing"
)

// freePortAddr returns the address of an OS-assigned free TCP port on the
// loopback interface, closing the listener so the port can be reused by the
// server under test.
func freePortAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	return addr
}
