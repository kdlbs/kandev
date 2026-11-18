package mcp

import (
	"context"
	"errors"
	"sync"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

var ErrStaleProviderScope = errors.New("provider MCP scope is not active")
var ErrProviderScopeBusy = errors.New("provider MCP scope has active tool calls")

type builtinGenerationGate struct {
	mu       sync.Mutex
	accepted bool
	retired  bool
	active   int
	scope    *p.ProviderMCPScope
}

func (g *builtinGenerationGate) begin() (func(), error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.accepted || g.retired {
		return nil, ErrStaleProviderScope
	}
	g.active++
	return sync.OnceFunc(func() { g.mu.Lock(); g.active--; g.mu.Unlock() }), nil
}

func (g *builtinGenerationGate) retire() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.retired {
		return nil
	}
	if g.active != 0 {
		return ErrProviderScopeBusy
	}
	g.retired = true
	g.accepted = false
	return nil
}

func (g *builtinGenerationGate) accept() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.retired {
		return ErrStaleProviderScope
	}
	g.accepted = true
	return nil
}

func (g *builtinGenerationGate) isRetired() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.retired
}

func (s *Server) beginGenerationEffect(_ context.Context) (func(), error) {
	if s.generationGate == nil {
		return func() {}, nil
	}
	return s.generationGate.begin()
}

func (s *Server) generationContext(ctx context.Context) context.Context {
	if s.generationGate == nil {
		return ctx
	}
	if scope := s.generationGate.scope; scope != nil {
		return streams.WithProviderMCPScope(ctx, *scope)
	}
	return ctx
}

func (g *builtinGenerationGate) freeze() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.retired {
		return ErrStaleProviderScope
	}
	if g.active != 0 {
		return ErrProviderScopeBusy
	}
	g.accepted = false
	return nil
}

func (s *Server) FreezeBuiltinGeneration(id string) error {
	if id == "" {
		return s.generationGate.freeze()
	}
	child := s.builtinGenerationServer(id)
	if child == nil {
		return ErrStaleProviderScope
	}
	return child.generationGate.freeze()
}
func (s *Server) ResumeBuiltinGeneration(id string) error {
	if id == "" {
		return s.generationGate.accept()
	}
	child := s.builtinGenerationServer(id)
	if child == nil {
		return ErrStaleProviderScope
	}
	return child.generationGate.accept()
}
