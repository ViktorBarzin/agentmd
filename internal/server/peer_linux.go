//go:build linux

package server

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// peerUID finds the owner of the socket at the other end of a loopback TCP
// connection by reading /proc/net/tcp (or tcp6): the client's socket is the
// row whose local address is our remote address and whose remote address is
// our local address.
func peerUID(local, remote *net.TCPAddr) (uint32, bool, error) {
	files := []string{"/proc/net/tcp", "/proc/net/tcp6"}
	for _, file := range files {
		six := strings.HasSuffix(file, "6")
		want := procAddr(remote, six) + " " + procAddr(local, six)
		if want == " " {
			continue
		}
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 8 || fields[1]+" "+fields[2] != want {
				continue
			}
			uid, err := strconv.ParseUint(fields[7], 10, 32)
			f.Close()
			if err != nil {
				return 0, false, err
			}
			return uint32(uid), true, nil
		}
		f.Close()
	}
	return 0, false, fmt.Errorf("no socket for %s -> %s", remote, local)
}

// procAddr writes an address the way /proc/net/tcp does: the IP as
// little-endian 32-bit words in hex, then the port in hex.
func procAddr(a *net.TCPAddr, six bool) string {
	var ip []byte
	if six {
		ip = a.IP.To16()
	} else {
		ip = a.IP.To4()
	}
	if ip == nil {
		return ""
	}
	out := make([]byte, len(ip))
	for i := 0; i < len(ip); i += 4 {
		binary.LittleEndian.PutUint32(out[i:], binary.BigEndian.Uint32(ip[i:]))
	}
	return strings.ToUpper(hex.EncodeToString(out)) + ":" + fmt.Sprintf("%04X", a.Port)
}

const peerCheckAvailable = true
