package loginpty

import (
	"net"
	"sync"
	"testing"
	"time"
)

func newLoginIdleProxy(t *testing.T, target string, idleTimeout time.Duration) *loginIdleProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for login idle proxy: %v", err)
	}
	proxy := &loginIdleProxy{
		listener: listener,
		target:   target,
		timeout:  idleTimeout,
		done:     make(chan struct{}),
	}
	go proxy.serve()
	t.Cleanup(proxy.close)
	return proxy
}

type loginIdleProxy struct {
	listener net.Listener
	target   string
	timeout  time.Duration
	done     chan struct{}
	mu       sync.Mutex
	closed   bool
	client   net.Conn
	backend  net.Conn
	closeOne sync.Once
}

func (p *loginIdleProxy) address() string {
	return p.listener.Addr().String()
}

func (p *loginIdleProxy) serve() {
	defer close(p.done)
	client, err := p.listener.Accept()
	if err != nil {
		return
	}
	backend, err := net.Dial("tcp", p.target)
	if err != nil {
		_ = client.Close()
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		_ = client.Close()
		_ = backend.Close()
		return
	}
	p.client, p.backend = client, backend
	p.mu.Unlock()

	activity := make(chan struct{}, 1)
	ended := make(chan struct{})
	closeBoth := func() {
		p.closeOne.Do(func() {
			close(ended)
			_ = client.Close()
			_ = backend.Close()
		})
	}
	var copies sync.WaitGroup
	copies.Add(2)
	go func() {
		defer copies.Done()
		copyLoginProxy(client, backend, activity)
		closeBoth()
	}()
	go func() {
		defer copies.Done()
		copyLoginProxy(backend, client, nil)
		closeBoth()
	}()
	timer := time.NewTimer(p.timeout)
	defer timer.Stop()
	for {
		select {
		case <-ended:
			copies.Wait()
			return
		case <-activity:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(p.timeout)
		case <-timer.C:
			closeBoth()
			copies.Wait()
			return
		}
	}
}

func copyLoginProxy(destination, source net.Conn, activity chan<- struct{}) {
	buffer := make([]byte, 32*1024)
	for {
		read, err := source.Read(buffer)
		if read > 0 {
			written, writeErr := destination.Write(buffer[:read])
			if activity != nil && written > 0 {
				select {
				case activity <- struct{}{}:
				default:
				}
			}
			if writeErr != nil || written != read {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (p *loginIdleProxy) close() {
	p.mu.Lock()
	p.closed = true
	_ = p.listener.Close()
	if p.client != nil {
		_ = p.client.Close()
	}
	if p.backend != nil {
		_ = p.backend.Close()
	}
	p.mu.Unlock()
	<-p.done
}
