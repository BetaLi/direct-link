package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

// Minimal SOCKS5 server for DirectLink relay.
// Supports: CONNECT method, username/password auth.
// Usage: ./socks5-relay -listen :10800 -user directlink -pass <password>

var (
	listenAddr = flag.String("listen", ":10800", "listen address")
	socksUser  = flag.String("user", "", "SOCKS5 username (empty = no auth)")
	socksPass  = flag.String("pass", "", "SOCKS5 password")
)

const (
	socks5Version  = 0x05
	authNoAuth     = 0x00
	authUserPass   = 0x02
	authNoneAvail  = 0xFF
	cmdConnect     = 0x01
	atypIPv4       = 0x01
	atypDomain     = 0x03
	atypIPv6       = 0x04
	replySuccess   = 0x00
	replyFailure   = 0x01
)

func main() {
	flag.Parse()

	ln, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}

	authMode := "no-auth"
	if *socksUser != "" {
		authMode = fmt.Sprintf("user=%s", *socksUser)
	}
	log.Printf("DirectLink SOCKS5 relay started on %s (%s)", *listenAddr, authMode)

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(60 * time.Second))

	// Step 1: Version + auth method negotiation
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return
	}
	if buf[0] != socks5Version {
		return
	}
	nMethods := int(buf[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}

	// Pick auth method
	needAuth := *socksUser != ""
	if needAuth {
		// Check if client supports username/password (0x02)
		supported := false
		for _, m := range methods {
			if m == authUserPass {
				supported = true
				break
			}
		}
		if !supported {
			conn.Write([]byte{socks5Version, authNoneAvail})
			return
		}
		// Send: use username/password auth
		conn.Write([]byte{socks5Version, authUserPass})

		// RFC 1929: username/password sub-negotiation
		ver := make([]byte, 2)
		if _, err := io.ReadFull(conn, ver); err != nil {
			return
		}
		// ver[0] = 0x01, ver[1] = ulen
		ulen := int(ver[1])
		user := make([]byte, ulen)
		if _, err := io.ReadFull(conn, user); err != nil {
			return
		}
		plenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, plenBuf); err != nil {
			return
		}
		pass := make([]byte, int(plenBuf[0]))
		if _, err := io.ReadFull(conn, pass); err != nil {
			return
		}
		if string(user) != *socksUser || string(pass) != *socksPass {
			conn.Write([]byte{0x01, 0x01}) // auth failure
			return
		}
		conn.Write([]byte{0x01, 0x00}) // auth success
	} else {
		conn.Write([]byte{socks5Version, authNoAuth})
	}

	// Step 2: Read CONNECT request
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	if header[0] != socks5Version || header[1] != cmdConnect {
		sendReply(conn, replyFailure)
		return
	}

	var target string
	switch header[3] {
	case atypIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return
		}
		target = net.IP(addr).String()
	case atypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return
		}
		domain := make([]byte, int(lenBuf[0]))
		if _, err := io.ReadFull(conn, domain); err != nil {
			return
		}
		target = string(domain)
	case atypIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return
		}
		target = net.IP(addr).String()
	default:
		sendReply(conn, replyFailure)
		return
	}

	// Read port
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return
	}
	port := int(portBuf[0])<<8 | int(portBuf[1])
	targetAddr := fmt.Sprintf("%s:%d", target, port)

	// Connect to target
	start := time.Now()
	upstream, err := net.DialTimeout("tcp", targetAddr, 15*time.Second)
	if err != nil {
		log.Printf("✗ connect %s failed: %v", targetAddr, err)
		sendReply(conn, replyFailure)
		return
	}

	// Log successful connection with client and target info
	clientAddr := conn.RemoteAddr().String()
	log.Printf("✓ %s → %s (%v)", clientAddr, targetAddr, time.Since(start))

	// Success reply
	sendReply(conn, replySuccess)

	// Reset deadline — tunnel established
	conn.SetDeadline(time.Time{})

	// Bidirectional pipe
	done := make(chan struct{}, 2)
	go func() { io.Copy(upstream, conn); done <- struct{}{} }()
	go func() { io.Copy(conn, upstream); done <- struct{}{} }()
	<-done
	upstream.Close()
}

func sendReply(conn net.Conn, code byte) {
	// SOCKS5 reply: version, reply, reserved, address type, bind addr, bind port
	conn.Write([]byte{
		socks5Version,
		code,
		0x00, // reserved
		atypIPv4,
		0, 0, 0, 0, // bind address (0.0.0.0)
		0, 0, // bind port (0)
	})
}

func init() {
	// Log to file if LOG_FILE is set
	if f := os.Getenv("LOG_FILE"); f != "" {
		lf, _ := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if lf != nil {
			log.SetOutput(lf)
		}
	}
	log.SetPrefix(strings.Repeat("", 0))
}
