// This helper runs only inside the darwin conformance suite. Each invocation
// performs one operation and exits 0 on success, 10 on EPERM or EACCES, and 11
// on any other error. It never prints a path, file content, or secret value.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	exitOK     = 0
	exitDenied = 10
	exitError  = 11
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(exitError)
	}
	os.Exit(code(run(os.Args[1], os.Args[2:])))
}

func code(err error) int {
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, syscall.EPERM), errors.Is(err, syscall.EACCES), errors.Is(err, errDenied):
		return exitDenied
	default:
		return exitError
	}
}

var errDenied = errors.New("denied")

func run(op string, args []string) error {
	switch op {
	case "read":
		return readOne(args[0])
	case "list":
		_, err := os.ReadDir(args[0])
		return err
	case "write":
		return os.WriteFile(args[0], []byte("canary"), 0o600)
	case "dial":
		conn, err := net.DialTimeout("tcp", args[0], 2*time.Second)
		if conn != nil {
			_ = conn.Close()
		}
		return err
	case "keychain":
		return keychain(args[0], args[1])
	case "proxy-post":
		return proxyPost(args[0])
	case "stdio":
		return stdio(args)
	case "post-direct":
		return post(http.Client{Timeout: 4 * time.Second}, args[0])
	case "group":
		return json.NewEncoder(os.Stdout).Encode(map[string]int{"pgid": syscall.Getpgrp(), "pid": os.Getpid()})
	}
	return errors.New("unknown operation")
}

func readOne(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = file.Read(make([]byte, 1))
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// keychain asks for one item of a throwaway keychain file. The tool's output is
// discarded, and a failure maps to a permission error: the caller states that a
// control obtained the item, so a sandboxed miss is a denial.
func keychain(file, service string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", service, "-w", file)
	command.Stdout, command.Stderr = nil, nil
	if err := command.Run(); err != nil {
		return errDenied
	}
	return nil
}

func proxyPost(target string) error {
	proxy, err := url.Parse(os.Getenv("HTTP_PROXY"))
	if err != nil || proxy.Host == "" {
		return errors.New("no proxy")
	}
	return post(http.Client{Timeout: 4 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}}, target)
}

func post(client http.Client, target string) error {
	response, err := client.Post(target, "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, response.Body)
	switch response.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusForbidden:
		return errDenied
	}
	return errors.New("unexpected status")
}

// stdio answers one JSON line on stdout. With a path argument it first attempts
// a read of that path, so a denial happens while the stream is open.
func stdio(args []string) error {
	line, err := bufio.NewReader(os.Stdin).ReadBytes('\n')
	if err != nil {
		return err
	}
	var request struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(line, &request); err != nil {
		return err
	}
	denied := false
	if len(args) > 0 {
		denied = errors.Is(readOne(args[0]), syscall.EPERM) || errors.Is(readOne(args[0]), syscall.EACCES)
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"denied": denied}})
}
