package main

import (
	"bytes"
	"net"
	"strconv"
	"time"
)

func ServerUp(address string, port int) (time.Duration, bool, error) {
	conn, err := net.DialTimeout("udp", net.JoinHostPort(address, fmtPort(port)), 3*time.Second)
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return 0, false, nil
		}
		return 0, false, nil
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return 0, false, err
	}
	if _, err := conn.Write([]byte{0x4f, 0x45, 0x74, 0x03, 0x00, 0x00, 0x00, 0x01}); err != nil {
		return 0, false, nil
	}

	start := time.Now()
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	elapsed := time.Since(start)
	if err != nil {
		return 0, false, nil
	}
	data := buf[:n]
	if len(data) < 14 || !bytes.Equal(data[0:4], []byte{0x4f, 0x45, 0x74, 0x03}) || !bytes.Equal(data[4:6], []byte{0x00, 0x01}) || data[11] != 0x01 {
		return 0, false, nil
	}

	peerID := data[12:14]
	disco := []byte{0x4f, 0x45, 0x74, 0x03, peerID[0], peerID[1], 0x00, 0x00, 0x03}
	_, _ = conn.Write(disco)
	return elapsed, true, nil
}

func fmtPort(port int) string {
	return strconv.Itoa(port)
}
