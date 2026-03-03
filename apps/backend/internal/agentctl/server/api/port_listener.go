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
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ListeningPort represents a TCP port with an active listener.
type ListeningPort struct {
	Port    int    `json:"port"`
	Address string `json:"address"`
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
		return detectFromProcNet()
	}
	return detectFromLsof()
}

// detectFromProcNet parses /proc/net/tcp for LISTEN state (0A) sockets.
func detectFromProcNet() ([]ListeningPort, error) {
	file, err := os.Open("/proc/net/tcp")
	if err != nil {
		return detectFromLsof() // fallback
	}
	defer func() { _ = file.Close() }()

	seen := make(map[int]bool)
	var ports []ListeningPort

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
	return ports, scanner.Err()
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
	if err != nil || len(b) != 4 {
		return "0.0.0.0"
	}
	// /proc/net/tcp stores in little-endian
	return fmt.Sprintf("%d.%d.%d.%d", b[3], b[2], b[1], b[0])
}

// detectFromLsof uses lsof as a fallback for non-Linux systems.
func detectFromLsof() ([]ListeningPort, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "lsof", "-iTCP", "-sTCP:LISTEN", "-nP", "-F", "n").Output()
	if err != nil {
		return nil, fmt.Errorf("lsof failed: %w", err)
	}

	seen := make(map[int]bool)
	var ports []ListeningPort

	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "n") {
			continue
		}
		// Format: n*:PORT or n127.0.0.1:PORT or n[::]:PORT
		addr := line[1:] // strip 'n' prefix
		_, portStr, err := net.SplitHostPort(addr)
		if err != nil {
			continue
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1024 || seen[port] {
			continue
		}
		seen[port] = true
		host, _, _ := net.SplitHostPort(addr)
		ports = append(ports, ListeningPort{Port: port, Address: host})
	}
	return ports, nil
}
