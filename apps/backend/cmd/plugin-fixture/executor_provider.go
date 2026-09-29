package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

const (
	executorProviderKey                  = "remote-sandbox"
	fixtureExecutorStateFileName         = "executor-resources.json"
	fixtureExecutorTrafficFile           = "executor-traffic.jsonl"
	fixtureExecutorBarrierEnv            = "KANDEV_PLUGIN_FIXTURE_EXECUTOR_BARRIER"
	fixtureBarrierLoseProvisionReplyOnce = "lose-provision-reply-once"
	fixtureBarrierUnavailableLeaseOnce   = "unavailable-lease-once"
	fixtureBarrierUnconfirmedCleanupOnce = "unconfirmed-cleanup-once"
	fixtureExecutorLifetime              = 8 * time.Hour
	fixtureLeaseLifetime                 = 2 * time.Minute
	fixtureRuntimePort                   = 8765
)

var _ pluginsdk.ExecutorProviderPlugin = (*fixturePlugin)(nil)

type fixtureExecutorState struct {
	Environments map[string]fixtureExecutorResource `json:"environments"`
	Allocations  int                                `json:"allocations"`
}

type fixtureExecutorResource struct {
	OperationID     string `json:"operation_id"`
	InputDigest     string `json:"input_digest"`
	EnvironmentID   string `json:"environment_id"`
	ResourceHandle  string `json:"resource_handle"`
	ExpiresAt       string `json:"expires_at"`
	ConnectionCount uint64 `json:"connection_count"`
}

type fixtureExecutorTrafficRecord struct {
	Method       string `json:"method"`
	Path         string `json:"path"`
	Generation   uint64 `json:"generation"`
	CredentialOK bool   `json:"credential_ok"`
}

type fixtureExecutorTransport struct {
	server      *http.Server
	listener    net.Listener
	endpoint    string
	certificate *x509.Certificate
	tokenMu     sync.RWMutex
	token       string
	generation  uint64
	dataDir     string
}

func fixtureExecutorCapabilities() *pluginsdk.ExecutorProviderCapabilities {
	return &pluginsdk.ExecutorProviderCapabilities{
		Terminal: true, Files: true, Git: true, EmbeddedEditor: true,
		Preview: true, Reattach: true, Retention: "bounded", MaximumLifetimeSeconds: uint64(fixtureExecutorLifetime.Seconds()),
	}
}

func (p *fixturePlugin) ValidateExecutorProfile(_ context.Context, req *pluginsdk.ValidateExecutorProfileRequest) (*pluginsdk.ValidateExecutorProfileResponse, error) {
	if req == nil || req.GetProfile() == nil {
		return nil, errors.New("plugin-fixture: missing executor profile")
	}
	var fieldErrors []*pluginsdk.ExecutorProviderFieldError
	if strings.TrimSpace(req.GetProfile().GetConfig()["region"]) == "" {
		fieldErrors = append(fieldErrors, &pluginsdk.ExecutorProviderFieldError{Field: "region", Code: "required", MessageId: "profile.region.required"})
	}
	return &pluginsdk.ValidateExecutorProfileResponse{FieldErrors: fieldErrors, Capabilities: fixtureExecutorCapabilities()}, nil
}

func (p *fixturePlugin) ProvisionExecutorEnvironment(ctx context.Context, req *pluginsdk.ProvisionExecutorEnvironmentRequest) (*pluginsdk.ProvisionExecutorEnvironmentResponse, error) {
	if err := validateFixtureExecutorRequest(req.GetContext()); err != nil {
		return nil, err
	}
	if req.GetProfile() == nil || req.GetProfile().GetProfileId() == "" {
		return nil, errors.New("plugin-fixture: missing executor profile identity")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resource, err := p.allocateFixtureExecutor(req.GetContext())
	if err != nil {
		return nil, err
	}
	if consumeFixtureFailureBarrier(p.dataDir, req.GetContext().GetOperationId(), fixtureBarrierLoseProvisionReplyOnce) {
		return nil, errors.New("plugin-fixture: fixture provision reply lost after allocation")
	}
	if _, err := p.ensureFixtureTransport(resource); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &pluginsdk.ProvisionExecutorEnvironmentResponse{Resource: fixtureExecutorDescriptor(resource)}, nil
}

func (p *fixturePlugin) RecoverExecutorOperation(ctx context.Context, req *pluginsdk.RecoverExecutorOperationRequest) (*pluginsdk.RecoverExecutorOperationResponse, error) {
	if err := validateFixtureExecutorRequest(req.GetContext()); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resource, found, err := p.fixtureResourceByOperation(req.GetContext())
	if err != nil || !found {
		return &pluginsdk.RecoverExecutorOperationResponse{Outcome: "absent"}, err
	}
	if _, err := p.ensureFixtureTransport(resource); err != nil {
		return nil, err
	}
	return &pluginsdk.RecoverExecutorOperationResponse{Outcome: "found", Resource: fixtureExecutorDescriptor(resource)}, nil
}

func (p *fixturePlugin) AttachExecutorEnvironment(ctx context.Context, req *pluginsdk.AttachExecutorEnvironmentRequest) (*pluginsdk.AttachExecutorEnvironmentResponse, error) {
	resource, found, err := p.fixtureResourceByHandle(req.GetResource().GetResourceHandle())
	if err != nil || !found {
		return &pluginsdk.AttachExecutorEnvironmentResponse{Error: fixtureProviderError("resource_not_found", "provider.fixture.notFound")}, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := p.ensureFixtureTransport(resource); err != nil {
		return nil, err
	}
	return &pluginsdk.AttachExecutorEnvironmentResponse{Resource: fixtureExecutorDescriptor(resource)}, nil
}

func (p *fixturePlugin) InspectExecutorEnvironment(ctx context.Context, req *pluginsdk.InspectExecutorEnvironmentRequest) (*pluginsdk.InspectExecutorEnvironmentResponse, error) {
	resource, found, err := p.fixtureResourceByHandle(req.GetResource().GetResourceHandle())
	if err != nil || !found {
		return &pluginsdk.InspectExecutorEnvironmentResponse{State: "absent", Reason: "resource_not_found"}, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &pluginsdk.InspectExecutorEnvironmentResponse{State: "running", ExpiresAt: resource.ExpiresAt}, nil
}

func (p *fixturePlugin) ResolveExecutorConnection(ctx context.Context, req *pluginsdk.ResolveExecutorConnectionRequest) (*pluginsdk.ResolveExecutorConnectionResponse, error) {
	if req.GetPurpose() != "agentctl" || req.GetRuntimePort() != fixtureRuntimePort {
		return &pluginsdk.ResolveExecutorConnectionResponse{Error: fixtureProviderError("unsupported_connection", "provider.fixture.unsupportedConnection")}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if consumeFixtureFailureBarrier(p.dataDir, req.GetContext().GetOperationId(), fixtureBarrierUnavailableLeaseOnce) {
		return &pluginsdk.ResolveExecutorConnectionResponse{Error: fixtureProviderError("connection_unavailable", "provider.fixture.connectionUnavailable")}, nil
	}
	resource, found, err := p.fixtureResourceByHandle(req.GetResource().GetResourceHandle())
	if err != nil || !found {
		return &pluginsdk.ResolveExecutorConnectionResponse{Error: fixtureProviderError("resource_not_found", "provider.fixture.notFound")}, err
	}
	transport, err := p.ensureFixtureTransport(resource)
	if err != nil {
		return nil, err
	}
	token, generation, err := p.rotateFixtureLease(resource)
	if err != nil {
		return nil, err
	}
	transport.setLease(token, generation)
	return &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{
		BaseUrl:          transport.endpoint,
		HttpHeaders:      map[string]string{"X-Fixture-Token": token},
		WebsocketHeaders: map[string]string{"X-Fixture-Token": token},
		ExpiresAt:        time.Now().UTC().Add(fixtureLeaseLifetime).Format(time.RFC3339Nano),
		Generation:       fmt.Sprintf("fixture-%d", generation),
	}}, nil
}

func (p *fixturePlugin) DestroyExecutorEnvironment(ctx context.Context, req *pluginsdk.DestroyExecutorEnvironmentRequest) (*pluginsdk.DestroyExecutorEnvironmentResponse, error) {
	if req.GetResource() == nil || req.GetResource().GetResourceHandle() == "" {
		return &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if consumeFixtureFailureBarrier(p.dataDir, req.GetContext().GetOperationId(), fixtureBarrierUnconfirmedCleanupOnce) {
		return &pluginsdk.DestroyExecutorEnvironmentResponse{Error: fixtureProviderError("cleanup_unconfirmed", "provider.fixture.cleanupUnconfirmed")}, nil
	}
	resource, found, err := p.fixtureResourceByHandle(req.GetResource().GetResourceHandle())
	if err != nil || !found {
		return &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}, err
	}
	p.executorMu.Lock()
	transport := p.executorTransports[resource.ResourceHandle]
	delete(p.executorTransports, resource.ResourceHandle)
	delete(p.executorState.Environments, resource.OperationID)
	err = p.saveExecutorStateLocked()
	p.executorMu.Unlock()
	if transport != nil {
		_ = transport.close(ctx)
	}
	if err != nil {
		return nil, err
	}
	return &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}, nil
}

func (p *fixturePlugin) allocateFixtureExecutor(request *pluginsdk.ExecutorProviderRequestContext) (fixtureExecutorResource, error) {
	p.executorMu.Lock()
	defer p.executorMu.Unlock()
	if p.executorStateErr != nil {
		return fixtureExecutorResource{}, p.executorStateErr
	}
	if p.executorState.Environments == nil {
		p.executorState.Environments = make(map[string]fixtureExecutorResource)
	}
	if existing, ok := p.executorState.Environments[request.GetOperationId()]; ok {
		if existing.EnvironmentID != request.GetEnvironmentId() || existing.InputDigest != request.GetInputDigest() {
			return fixtureExecutorResource{}, errors.New("plugin-fixture: operation identity changed after allocation")
		}
		return existing, nil
	}
	resource := fixtureExecutorResource{
		OperationID: request.GetOperationId(), InputDigest: request.GetInputDigest(),
		EnvironmentID: request.GetEnvironmentId(), ResourceHandle: "fixture:" + request.GetEnvironmentId(),
		ExpiresAt: time.Now().UTC().Add(fixtureExecutorLifetime).Format(time.RFC3339Nano),
	}
	p.executorState.Environments[resource.OperationID] = resource
	p.executorState.Allocations++
	if err := p.saveExecutorStateLocked(); err != nil {
		delete(p.executorState.Environments, resource.OperationID)
		p.executorState.Allocations--
		return fixtureExecutorResource{}, err
	}
	return resource, nil
}

func (p *fixturePlugin) fixtureResourceByOperation(request *pluginsdk.ExecutorProviderRequestContext) (fixtureExecutorResource, bool, error) {
	p.executorMu.Lock()
	defer p.executorMu.Unlock()
	if p.executorStateErr != nil {
		return fixtureExecutorResource{}, false, p.executorStateErr
	}
	resource, found := p.executorState.Environments[request.GetOperationId()]
	if found && (resource.EnvironmentID != request.GetEnvironmentId() || resource.InputDigest != request.GetInputDigest()) {
		return fixtureExecutorResource{}, false, errors.New("plugin-fixture: recovered operation identity does not match")
	}
	return resource, found, nil
}

func (p *fixturePlugin) fixtureResourceByHandle(handle string) (fixtureExecutorResource, bool, error) {
	p.executorMu.Lock()
	defer p.executorMu.Unlock()
	if p.executorStateErr != nil {
		return fixtureExecutorResource{}, false, p.executorStateErr
	}
	for _, resource := range p.executorState.Environments {
		if resource.ResourceHandle == handle {
			return resource, true, nil
		}
	}
	return fixtureExecutorResource{}, false, nil
}

func (p *fixturePlugin) ensureFixtureTransport(resource fixtureExecutorResource) (*fixtureExecutorTransport, error) {
	p.executorMu.Lock()
	defer p.executorMu.Unlock()
	if p.executorTransports == nil {
		p.executorTransports = make(map[string]*fixtureExecutorTransport)
	}
	if transport := p.executorTransports[resource.ResourceHandle]; transport != nil {
		return transport, nil
	}
	transport, err := newFixtureExecutorTransport(p.dataDir)
	if err != nil {
		return nil, err
	}
	p.executorTransports[resource.ResourceHandle] = transport
	return transport, nil
}

func (p *fixturePlugin) rotateFixtureLease(resource fixtureExecutorResource) (string, uint64, error) {
	p.executorMu.Lock()
	defer p.executorMu.Unlock()
	current, found := p.executorState.Environments[resource.OperationID]
	if !found || current.ResourceHandle != resource.ResourceHandle {
		return "", 0, errors.New("plugin-fixture: connection lease resource is not active")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", 0, errors.New("plugin-fixture: connection lease could not be generated")
	}
	current.ConnectionCount++
	p.executorState.Environments[resource.OperationID] = current
	if err := p.saveExecutorStateLocked(); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(secret), current.ConnectionCount, nil
}

func (p *fixturePlugin) saveExecutorStateLocked() error {
	if err := os.MkdirAll(p.dataDir, 0o700); err != nil {
		return fmt.Errorf("plugin-fixture: create provider inventory: %w", err)
	}
	data, err := json.Marshal(p.executorState)
	if err != nil {
		return errors.New("plugin-fixture: encode provider inventory")
	}
	path := filepath.Join(p.dataDir, fixtureExecutorStateFileName)
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("plugin-fixture: write provider inventory: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("plugin-fixture: commit provider inventory: %w", err)
	}
	return nil
}

func (p *fixturePlugin) executorAllocationCount() int {
	p.executorMu.Lock()
	defer p.executorMu.Unlock()
	return p.executorState.Allocations
}

func (p *fixturePlugin) certificateForLease(lease *pluginsdk.ExecutorConnectionLease) *x509.Certificate {
	p.executorMu.Lock()
	defer p.executorMu.Unlock()
	for _, transport := range p.executorTransports {
		if transport.endpoint == lease.GetBaseUrl() {
			return transport.certificate
		}
	}
	return nil
}

func (p *fixturePlugin) Close() error {
	p.executorMu.Lock()
	transports := make([]*fixtureExecutorTransport, 0, len(p.executorTransports))
	for _, transport := range p.executorTransports {
		transports = append(transports, transport)
	}
	p.executorTransports = make(map[string]*fixtureExecutorTransport)
	p.executorMu.Unlock()
	var closeErrors []error
	for _, transport := range transports {
		if err := transport.close(context.Background()); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	return errors.Join(closeErrors...)
}

func (p *fixtureExecutorTransport) setLease(token string, generation uint64) {
	p.tokenMu.Lock()
	p.token = token
	p.generation = generation
	p.tokenMu.Unlock()
}

func (p *fixtureExecutorTransport) currentLease() (string, uint64) {
	p.tokenMu.RLock()
	defer p.tokenMu.RUnlock()
	return p.token, p.generation
}

func (p *fixtureExecutorTransport) close(ctx context.Context) error {
	if p.server == nil {
		return nil
	}
	return p.server.Shutdown(ctx)
}

func newFixtureExecutorTransport(dataDir string) (*fixtureExecutorTransport, error) {
	address, err := fixtureAdvertisedAddress()
	if err != nil {
		return nil, err
	}
	certificate, parsedCertificate, err := fixtureExecutorTLSCertificate(address)
	if err != nil {
		return nil, err
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: parsedCertificate.Raw})
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("plugin-fixture: create HTTPS test certificate directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "executor-test-ca.pem"), certificatePEM, 0o600); err != nil {
		return nil, fmt.Errorf("plugin-fixture: persist HTTPS test certificate: %w", err)
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("0.0.0.0", "0"))
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: start HTTPS listener: %w", err)
	}
	transport := &fixtureExecutorTransport{listener: listener, certificate: parsedCertificate, dataDir: dataDir}
	transport.endpoint = "https://" + net.JoinHostPort(address.String(), strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
	mux := http.NewServeMux()
	mux.HandleFunc("/health", transport.handleHealth)
	mux.HandleFunc("/echo", transport.handleEcho)
	transport.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	config := &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	go func() {
		_ = transport.server.Serve(tls.NewListener(listener, config))
	}()
	return transport, nil
}

func (p *fixtureExecutorTransport) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if !p.authorize(writer, request) {
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write([]byte(`{"state":"running"}`))
}

func (p *fixtureExecutorTransport) handleEcho(writer http.ResponseWriter, request *http.Request) {
	if !p.authorize(writer, request) {
		return
	}
	_, _ = fmt.Fprint(writer, request.URL.Query().Get("value"))
}

func (p *fixtureExecutorTransport) authorize(writer http.ResponseWriter, request *http.Request) bool {
	token, generation := p.currentLease()
	provided := request.Header.Get("X-Fixture-Token")
	valid := token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(provided)) == 1
	if err := appendJSONLine(filepath.Join(p.dataDir, fixtureExecutorTrafficFile), fixtureExecutorTrafficRecord{
		Method: request.Method, Path: request.URL.Path, Generation: generation, CredentialOK: valid,
	}); err != nil {
		http.Error(writer, "fixture request record unavailable", http.StatusInternalServerError)
		return false
	}
	if !valid {
		http.Error(writer, "connection lease expired", http.StatusUnauthorized)
		return false
	}
	return true
}

func fixtureExecutorTLSCertificate(address net.IP) (tls.Certificate, *x509.Certificate, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, errors.New("plugin-fixture: generate HTTPS key")
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return tls.Certificate{}, nil, errors.New("plugin-fixture: generate HTTPS certificate serial")
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "plugin-fixture-executor"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true, IsCA: true, IPAddresses: []net.IP{address},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, nil, errors.New("plugin-fixture: create HTTPS certificate")
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, nil, errors.New("plugin-fixture: parse HTTPS certificate")
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: privateKey, Leaf: certificate}, certificate, nil
}

func fixtureAdvertisedAddress() (net.IP, error) {
	connection, err := net.DialTimeout("udp4", "1.1.1.1:53", time.Second)
	if err == nil {
		address := connection.LocalAddr().(*net.UDPAddr).IP.To4()
		_ = connection.Close()
		if isFixtureDialableAddress(address) {
			return address, nil
		}
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, errors.New("plugin-fixture: resolve HTTPS listener address")
	}
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, addressErr := networkInterface.Addrs()
		if addressErr != nil {
			continue
		}
		for _, item := range addresses {
			ip, _, parseErr := net.ParseCIDR(item.String())
			if parseErr == nil && isFixtureDialableAddress(ip.To4()) {
				return ip.To4(), nil
			}
		}
	}
	return nil, errors.New("plugin-fixture: no non-loopback IPv4 address is available")
}

func isFixtureDialableAddress(address net.IP) bool {
	return address != nil && address.To4() != nil && !address.IsLoopback() && !address.IsUnspecified() && !address.IsLinkLocalUnicast() && !address.IsMulticast()
}

func fixtureExecutorDescriptor(resource fixtureExecutorResource) *pluginsdk.ExecutorResourceDescriptor {
	state, _ := json.Marshal(map[string]string{"instance_id": "fixture-remote-sandbox"})
	return &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: resource.ResourceHandle, StateJson: string(state), Platform: "linux-amd64",
		Capabilities: fixtureExecutorCapabilities(), ExpiresAt: resource.ExpiresAt,
		Retention: "bounded", StateVersion: 1,
	}
}

func validateFixtureExecutorRequest(request *pluginsdk.ExecutorProviderRequestContext) error {
	if request == nil || request.GetOperationId() == "" || request.GetEnvironmentId() == "" || request.GetInputDigest() == "" {
		return errors.New("plugin-fixture: executor operation identity is incomplete")
	}
	return nil
}

func fixtureProviderError(code, messageID string) *pluginsdk.ExecutorProviderError {
	return &pluginsdk.ExecutorProviderError{Code: code, MessageId: messageID}
}

func consumeFixtureFailureBarrier(dataDir, operationID, barrier string) bool {
	if os.Getenv(fixtureExecutorBarrierEnv) != barrier || operationID == "" {
		return false
	}
	hash := sha256.Sum256([]byte(operationID + "\x00" + barrier))
	path := filepath.Join(dataDir, ".barrier-"+hex.EncodeToString(hash[:8]))
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return false
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	_ = file.Close()
	return true
}
