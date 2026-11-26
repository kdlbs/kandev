package acpv2

import (
	"context"
	"errors"
	"io"
	"sync"

	acp "github.com/coder/acp-go-sdk"
	wire "github.com/coder/acp-go-sdk/experimental/v2"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

type AgentInfo struct{ Name, Version string }

// Adapter holds the protocol state of one ACP v2 subprocess.
type Adapter struct {
	cfg *shared.Config
	log *logger.Logger

	ctx     context.Context
	cancel  context.CancelFunc
	closing sync.Once
	emitMu  sync.RWMutex
	updates chan streams.AgentEvent

	// projectMu keeps projected events ordered across state changes and channel delivery.
	projectMu sync.Mutex
	// commands serializes setup and configuration RPCs with prompt admission.
	// Notifications and server requests use mu and can run during those RPCs.
	commands          chan struct{}
	mu                sync.Mutex
	client            *wire.ClientSideConnection
	info              *wire.InitializeResponse
	session           wire.SessionID
	attach            *sessionAttachment
	state             projection
	blocked           bool
	turn              *promptTurn
	settings          []streams.ConfigOption
	settingsComplete  bool
	verifiedOptionIDs []string
	revision          uint64
	approval          types.PermissionHandler
	input             types.UserInputRequestHandler
}

func NewAdapter(cfg *shared.Config, log *logger.Logger) *Adapter {
	ctx, cancel := context.WithCancel(context.Background())
	return &Adapter{
		cfg: cfg, log: log, ctx: ctx, cancel: cancel,
		commands: make(chan struct{}, 1), updates: make(chan streams.AgentEvent, 128),
	}
}

func (*Adapter) PrepareEnvironment() (map[string]string, error) { return nil, nil }
func (*Adapter) PrepareCommandArgs() []string                   { return nil }

func (a *Adapter) Connect(stdin io.Writer, stdout io.Reader) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client != nil || a.ctx.Err() != nil {
		return errors.New("ACP v2 adapter cannot be connected again")
	}
	a.client = wire.NewClientSideConnection(wire.ClientHandlers{
		SessionUpdate:     a.handleUpdate,
		RequestPermission: a.requestPermission,
		CreateElicitation: a.createElicitation,
	}, stdin, stdout, acp.WithMaxQueuedNotifications(a.cfg.NotificationQueueCapacity))
	return nil
}

func (a *Adapter) acquire(ctx context.Context) error {
	select {
	case a.commands <- struct{}{}:
		if err := ctx.Err(); err != nil {
			a.release()
			return err
		}
		if err := a.ctx.Err(); err != nil {
			a.release()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-a.ctx.Done():
		return a.ctx.Err()
	}
}

func (a *Adapter) release() { <-a.commands }

func (a *Adapter) Initialize(ctx context.Context) error {
	if err := a.acquire(ctx); err != nil {
		return err
	}
	defer a.release()
	a.mu.Lock()
	client, initialized, hasInput := a.client, a.info != nil, a.input != nil
	a.mu.Unlock()
	if initialized {
		return nil
	}
	if client == nil {
		return errors.New("ACP v2 adapter is disconnected")
	}
	request := wire.InitializeRequest{
		ProtocolVersion: 2, Info: wire.Implementation{Name: "kandev", Version: "1.0.0"},
	}
	if hasInput {
		request.Capabilities = &wire.ClientCapabilities{
			Elicitation: &wire.ElicitationCapabilities{Form: &wire.Capability{}},
		}
	}
	ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.cfg.AgentID, "initialize")
	defer span.End()
	response, err := client.Initialize(ctx, request)
	if err != nil {
		var legacy *wire.LegacyPeerError
		if errors.As(err, &legacy) {
			return legacy
		}
		return errors.New("ACP v2 initialization failed")
	}
	a.mu.Lock()
	a.info = &response
	a.mu.Unlock()
	a.publishCapabilities()
	a.log.Info("ACP initialized", zap.String("agent_id", a.cfg.AgentID),
		zap.String("agent_version", response.Info.Version), zap.Uint16("requested_version", 2),
		zap.Uint16("selected_version", response.ProtocolVersion))
	return nil
}

func (a *Adapter) publishCapabilities() {
	event := streams.AgentEvent{
		Type: streams.EventTypeAgentCapabilities, AuthMethods: a.GetSessionOperationAuthMethods(),
	}
	if caps := a.promptCapabilities(); caps != nil {
		event.SupportsImage = caps.Image != nil
	}
	a.emit(event)
}

func (a *Adapter) GetAgentInfo() *AgentInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.info == nil {
		return nil
	}
	return &AgentInfo{Name: a.info.Info.Name, Version: a.info.Info.Version}
}

func (a *Adapter) ACPProtocolVersion() uint16 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.info == nil {
		return 0
	}
	return a.info.ProtocolVersion
}

func (a *Adapter) GetSessionID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return string(a.session)
}

func (a *Adapter) GetOperationID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.turn == nil {
		return ""
	}
	return a.turn.messageID
}

func (a *Adapter) SetPermissionHandler(handler types.PermissionHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.approval = handler
}

func (a *Adapter) SetUserInputRequestHandler(handler types.UserInputRequestHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.input = handler
}

func (a *Adapter) Updates() <-chan streams.AgentEvent { return a.updates }
func (a *Adapter) RequiresProcessKill() bool          { return a.cfg.RequiresProcessKill }

// OwnsPromptDisconnect keeps a process exit from settling the turn while
// the adapter is still deciding whether the correlated prompt was accepted.
func (a *Adapter) OwnsPromptDisconnect() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.turn != nil || a.blocked
}

func (*Adapter) SanitizeStderrLine(string) (string, bool) { return "", false }

type eventBatch struct {
	events  []streams.AgentEvent
	settled *promptTurn
}

func (b *eventBatch) add(event streams.AgentEvent) {
	b.events = append(b.events, event)
}

func (a *Adapter) project(update func(*eventBatch)) {
	a.projectMu.Lock()
	defer a.projectMu.Unlock()
	var batch eventBatch
	a.mu.Lock()
	update(&batch)
	a.mu.Unlock()
	for _, event := range batch.events {
		a.emit(event)
	}
	if turn := batch.settled; turn != nil {
		a.mu.Lock()
		a.turn = nil
		close(turn.done)
		a.mu.Unlock()
	}
}

func (a *Adapter) emit(event streams.AgentEvent) {
	a.emitMu.RLock()
	defer a.emitMu.RUnlock()
	if a.ctx.Err() != nil {
		return
	}
	select {
	case a.updates <- event:
	case <-a.ctx.Done():
	}
}

func (a *Adapter) Close() error {
	a.closing.Do(func() {
		a.cancel()
		a.emitMu.Lock()
		defer a.emitMu.Unlock()
		close(a.updates)
	})
	return nil
}
