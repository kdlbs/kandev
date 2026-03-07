package api

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const defaultListenAddr = "0.0.0.0"

// ListeningPort represents a TCP port with an active listener.
type ListeningPort struct {
	Port    int    `json:"port"`
	Address string `json:"address"`
	Process string `json:"process,omitempty"`
}

// handleListPorts returns all TCP ports currently listening inside the executor.
func (s *Server) handleListPorts(c *gin.Context) {
	ports, err := detectListeningPorts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ports": ports})
}

func detectListeningPorts() ([]ListeningPort, error) {
	if runtime.GOOS == "linux" {
		ports, err := detectFromSS()
		if err == nil {
			return ports, nil
		}
		return detectFromProcNet()
	}
	return detectFromLsof()
}

// processNameRe extracts the first process name from ss users field.
// Example: users:(("next-server (v1",pid=16186,fd=24)) → "next-server (v1"
var processNameRe = regexp.MustCompile(`\(\("([^"]+)"`)

// detectFromSS uses ss -tlnp to detect listening ports with process names.
// Covers both IPv4 and IPv6 sockets.
func detectFromSS() ([]ListeningPort, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "ss", "-tlnpH").Output()
	if err != nil {
		return nil, fmt.Errorf("ss failed: %w", err)
	}

	seen := make(map[int]bool)
	var ports []ListeningPort

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// Field 3 is local address:port (e.g. "0.0.0.0:3000", "*:8080", "[::]:3000")
		localAddr := fields[3]
		host, portStr, splitErr := net.SplitHostPort(localAddr)
		if splitErr != nil {
			continue
		}
		port, convErr := strconv.Atoi(portStr)
		if convErr != nil || port < 1024 || seen[port] {
			continue
		}
		seen[port] = true

		var process string
		// Process info is in the last field(s) — rejoin everything after peer address
		rest := strings.Join(fields[5:], " ")
		if match := processNameRe.FindStringSubmatch(rest); len(match) > 1 {
			process = match[1]
		}

		if host == "*" || host == "[::]" {
			host = defaultListenAddr
		}

		ports = append(ports, ListeningPort{Port: port, Address: host, Process: process})
	}
	return ports, nil
}

// detectFromProcNet parses /proc/net/tcp and /proc/net/tcp6 for LISTEN state (0A) sockets.
func detectFromProcNet() ([]ListeningPort, error) {
	seen := make(map[int]bool)
	var ports []ListeningPort

	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		file, err := os.Open(path)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(file)
		scanner.Scan() // skip header
		for scanner.Scan() {
			port, addr, ok := parseProcNetLine(scanner.Text())
			if !ok || port < 1024 || seen[port] {
				continue
			}
			seen[port] = true
			ports = append(ports, ListeningPort{Port: port, Address: addr})
		}
		_ = file.Close()
	}

	if len(ports) == 0 {
		return detectFromLsof()
	}
	return ports, nil
}

// parseProcNetLine extracts port and address from a /proc/net/tcp line.
// Format: sl local_address rem_address st ...
// local_address is hex IP:hex port, st=0A means LISTEN.
func parseProcNetLine(line string) (int, string, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return 0, "", false
	}
	// State must be LISTEN (0A)
	if fields[3] != "0A" {
		return 0, "", false
	}
	localAddr := fields[1]
	parts := strings.SplitN(localAddr, ":", 2)
	if len(parts) != 2 {
		return 0, "", false
	}
	portHex := parts[1]
	port64, err := strconv.ParseInt(portHex, 16, 32)
	if err != nil {
		return 0, "", false
	}
	port := int(port64)

	addr := parseHexIP(parts[0])
	return port, addr, true
}

func parseHexIP(hexIP string) string {
	b, err := hex.DecodeString(hexIP)
	if err != nil || len(b) < 4 {
		return defaultListenAddr
	}
	if len(b) == 4 {
		// IPv4: /proc/net/tcp stores in little-endian
		return fmt.Sprintf("%d.%d.%d.%d", b[3], b[2], b[1], b[0])
	}
	// IPv6: treat all addresses as all-interfaces for port detection purposes.
	return defaultListenAddr
}

// detectFromLsof uses lsof as a fallback for non-Linux systems.
func detectFromLsof() ([]ListeningPort, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "lsof", "-iTCP", "-sTCP:LISTEN", "-nP", "-F", "nc").Output()
	if err != nil {
		return nil, fmt.Errorf("lsof failed: %w", err)
	}

	seen := make(map[int]bool)
	var ports []ListeningPort
	var currentProcess string

	for _, line := range strings.Split(string(out), "\n") {
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case 'c':
			currentProcess = line[1:]
		case 'n':
			// Format: n*:PORT or n127.0.0.1:PORT or n[::]:PORT
			addr := line[1:]
			host, portStr, splitErr := net.SplitHostPort(addr)
			if splitErr != nil {
				continue
			}
			port, convErr := strconv.Atoi(portStr)
			if convErr != nil || port < 1024 || seen[port] {
				continue
			}
			seen[port] = true
			ports = append(ports, ListeningPort{Port: port, Address: host, Process: currentProcess})
		}
	}
	return ports, nil
}
