package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/sysprompt"
	"slices"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

type builtinGeneration struct {
	scope       p.ProviderMCPScope
	baseProfile p.Context
	secretHash  [32]byte
	server      *Server
	router      *gin.Engine
}

type builtinGenerations struct {
	mu      sync.RWMutex
	entries map[string]*builtinGeneration
	current string
}

// PrepareBuiltinGeneration creates an immutable catalog. Tool effects remain
// closed until the backend commits this exact scope and calls acceptance.
func (s *Server) PrepareBuiltinGeneration(scope p.ProviderMCPScope, secret string, profile p.Context) error {
	frozen, err := s.validateBuiltinGeneration(scope, secret, profile)
	if err != nil {
		return err
	}
	secretHash := sha256.Sum256([]byte(secret))
	s.generations.mu.Lock()
	defer s.generations.mu.Unlock()
	if prior := s.generations.entries[scope.BindingID]; prior != nil {
		if prior.scope == scope && prior.secretHash == secretHash {
			return nil
		}
		return errors.New("provider MCP binding ID is already used")
	}
	for _, prior := range s.generations.entries {
		if !prior.server.generationGate.isRetired() && prior.scope.BindingID != s.generations.current {
			return errors.New("a provider MCP generation is already prepared")
		}
		if prior.scope.ProcessGeneration >= scope.ProcessGeneration {
			return errors.New("provider MCP generation is not newer")
		}
	}
	child := NewWithProfile(s.backend, s.sessionID, s.taskID, 0, s.logger, "", false, *frozen, WithMCPToolNamespacingByServer(s.namespacesMCPToolsByServer), func(child *Server) {
		child.generationGate = &builtinGenerationGate{scope: &scope}
		identity := fmt.Sprintf("Kandev Task ID: %s. Kandev Session ID: %s. Provider MCP binding: %s. Keep the existing conversation, title ownership, question barriers, completion gates, autopilot rules, and delegation boundaries.", s.taskID, s.sessionID, scope.BindingID)
		child.providerInstructions = sysprompt.AppendBuiltinBindingGuidance(identity, frozen.BuiltinPackages)
	})
	router := gin.New()
	child.RegisterRoutes(router)
	if s.generations.entries == nil {
		s.generations.entries = make(map[string]*builtinGeneration)
	}
	s.generations.entries[scope.BindingID] = &builtinGeneration{scope: scope, baseProfile: *frozen, secretHash: secretHash, server: child, router: router}
	return nil
}

func (s *Server) builtinGenerationServer(id string) *Server {
	s.generations.mu.RLock()
	defer s.generations.mu.RUnlock()
	if entry := s.generations.entries[id]; entry != nil {
		return entry.server
	}
	return nil
}

func (s *Server) RetireBuiltinGeneration(id string) error {
	if id == "" {
		return s.generationGate.retire()
	}
	child := s.builtinGenerationServer(id)
	if child == nil {
		return ErrStaleProviderScope
	}
	return child.generationGate.retire()
}

func (s *Server) AcceptBuiltinGeneration(id string) error {
	s.generations.mu.Lock()
	defer s.generations.mu.Unlock()
	entry := s.generations.entries[id]
	if entry == nil {
		return ErrStaleProviderScope
	}
	if !s.generationGate.isRetired() {
		return errors.New("legacy provider MCP scope is still active")
	}
	if current := s.generations.entries[s.generations.current]; current != nil && current != entry && !current.server.generationGate.isRetired() {
		return errors.New("previous provider MCP scope is still active")
	}
	if err := entry.server.generationGate.accept(); err != nil {
		return err
	}
	s.generations.current = id
	return nil
}

func (s *Server) serveBuiltinGeneration(c *gin.Context) {
	s.generations.mu.RLock()
	entry := s.generations.entries[c.Param("bindingID")]
	s.generations.mu.RUnlock()
	if entry == nil {
		c.Status(404)
		return
	}
	if !strings.HasPrefix(c.GetHeader("Authorization"), "Bearer ") {
		c.Status(401)
		return
	}
	supplied := sha256.Sum256([]byte(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")))
	if subtle.ConstantTimeCompare(supplied[:], entry.secretHash[:]) != 1 {
		c.Status(401)
		return
	}
	if entry.server.generationGate.isRetired() {
		c.Status(410)
		return
	}
	req := c.Request.Clone(c.Request.Context())
	req.Header.Del("Authorization")
	path := *req.URL
	path.Path = "/mcp" + c.Param("packagePath")
	path.RawPath = ""
	req.URL = &path
	entry.router.ServeHTTP(c.Writer, req)
}

func (s *Server) allowGenerationTransport(c *gin.Context) bool {
	if s.generationGate.isRetired() {
		c.Status(410)
		return false
	}
	return true
}

func (s *Server) closeBuiltinGenerations(ctx context.Context) error {
	s.generations.mu.RLock()
	entries := make([]*Server, 0, len(s.generations.entries))
	for _, entry := range s.generations.entries {
		entries = append(entries, entry.server)
	}
	s.generations.mu.RUnlock()
	var errs []error
	for _, child := range entries {
		errs = append(errs, child.Close(ctx))
	}
	return errors.Join(errs...)
}

// BuiltinGenerationCatalog returns the exact transport names the provider must
// observe for this immutable scope, including provider namespacing rules.
func (s *Server) BuiltinGenerationCatalog(id string) (map[string][]string, error) {
	child := s.builtinGenerationServer(id)
	if child == nil {
		return nil, ErrStaleProviderScope
	}
	canonical := child.BuiltinCatalog()
	result := make(map[string][]string, len(canonical))
	for _, descriptor := range p.BuiltinPackageCatalog() {
		names, selected := canonical[descriptor.ID]
		if !selected {
			continue
		}
		if child.namespacesMCPToolsByServer {
			names = slices.Clone(names)
			for index, name := range names {
				short := strings.TrimSuffix(name, mcpToolNameSuffix)
				if !slices.Contains(canonical[descriptor.ID], short) {
					names[index] = short
				}
			}
			slices.Sort(names)
		}
		result[descriptor.RuntimeName] = names
	}
	return result, nil
}

func (s *Server) BuiltinGenerationProfile(id string) (p.Context, error) {
	s.generations.mu.RLock()
	defer s.generations.mu.RUnlock()
	entry := s.generations.entries[id]
	if entry == nil {
		return p.Context{}, ErrStaleProviderScope
	}
	return p.Normalize(entry.baseProfile), nil
}

func (s *Server) BuiltinGenerationToolCatalog(id string) map[p.PackageID][]string {
	child := s.builtinGenerationServer(id)
	if child == nil {
		return nil
	}
	return child.BuiltinCatalog()
}

func (s *Server) validateBuiltinGeneration(scope p.ProviderMCPScope, secret string, profile p.Context) (*p.Context, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if len(secret) < 32 || len(secret) > 256 {
		return nil, errors.New("provider MCP scope credential is invalid")
	}
	if s.generationGate.scope != nil {
		return nil, errors.New("nested provider MCP generations are not supported")
	}
	encoded, err := p.MarshalBuiltinBinding(profile)
	if err != nil {
		return nil, err
	}
	frozen, err := p.ParseBuiltinBindingMetadata(encoded)
	if err != nil {
		return nil, err
	}
	digest, err := p.BuiltinBindingDigest(*frozen)
	if err != nil {
		return nil, err
	}
	if digest != scope.ProfileDigest || frozen.BuiltinPackages.TaskID != s.taskID || s.Profile().BuiltinPackages == nil || frozen.BuiltinPackages.WorkspaceID != s.Profile().BuiltinPackages.WorkspaceID || (frozen.Surface != p.SurfaceKanbanTask && frozen.Surface != p.SurfaceOfficeTask) {
		return nil, errors.New("provider MCP scope profile does not match its owner")
	}

	return frozen, nil
}

func (s *Server) BuiltinGenerationRuntimeProfile(id string) (p.Context, error) {
	child := s.builtinGenerationServer(id)
	if child == nil {
		return p.Context{}, ErrStaleProviderScope
	}
	return child.Profile(), nil
}
func (s *Server) SetBuiltinGenerationMode(id, mode string) error {
	child := s.builtinGenerationServer(id)
	if child == nil || child.generationGate.isRetired() {
		return ErrStaleProviderScope
	}
	child.SetMode(mode)
	return nil
}
func (s *Server) SetBuiltinGenerationProviders(id string, providers []string) error {
	child := s.builtinGenerationServer(id)
	if child == nil || child.generationGate.isRetired() {
		return ErrStaleProviderScope
	}
	child.SetProviders(providers)
	return nil
}

// ValidateBuiltinGeneration checks a scope without creating its transport.
func (s *Server) ValidateBuiltinGeneration(scope p.ProviderMCPScope, credential string, profile p.Context) error {
	_, err := s.validateBuiltinGeneration(scope, credential, profile)
	return err
}
