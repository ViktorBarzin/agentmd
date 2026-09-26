//go:build !linux

package server

import "net"

// peerUID is only available on Linux; elsewhere every loopback connection is
// accepted, as documented in ADR-0002.
func peerUID(local, remote *net.TCPAddr) (uint32, bool, error) { return 0, false, nil }

const peerCheckAvailable = false
