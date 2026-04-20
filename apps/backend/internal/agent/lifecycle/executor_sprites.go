package lifecycle

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	sprites "github.com/superfly/sprites-go"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
)

type RemoteAuthAgentLister interface {
	ListEnabled() []agents.Agent
}

// spriteFileUploader implements FileUploader for a Sprites instance.
type spriteFileUploader struct {
	sprite  *sprites.Sprite
	runtime *SpritesExecutor
}

func (u *spriteFileUploader) WriteFile(ctx context.Context, path string, data []byte, mode os.FileMode) error {
	if u.runtime == nil {
		return u.sprite.Filesystem().WriteFileContext(ctx, path, data, mode)
	}
	return u.runtime.writeFileWithRetry(ctx, u.sprite, path, data, mode)
}

func (u *spriteFileUploader) RunCommand(ctx context.Context, name string, args ...string) error {
	_, err := u.sprite.CommandContext(ctx, name, args...).Output()
	return err
}

func (u *spriteFileUploader) RunCommandOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return u.sprite.CommandContext(ctx, name, args...).Output()
}

type commandOutputRunner interface {
	RunCommandOutput(ctx context.Context, name string, args ...string) ([]byte, error)
}

const (
	spritesAgentctlPort    = 8765
	spritesWorkspacePath   = "/workspace"
	spritesNamePrefix      = "kandev-"
	spriteStepTimeout      = 120 * time.Second
	spriteHealthTimeout    = 15 * time.Second
	spriteDestroyTimeout   = 30 * time.Second
	spriteHealthRetryWait  = 500 * time.Millisecond
	spritesTotalSteps      = 7 // create, upload, credentials, prepare script, health, create instance, network policy
	spriteOutputMaxLines   = 20
	spriteUploadMaxRetries = 3
)

// SpritesProxySession tracks an active port-forwarding session to a sprite.
type SpritesProxySession struct {
	spriteName   string
	localPort    int
	proxySession *sprites.ProxySession
	cancel       context.CancelFunc
}

// SpritesExecutor implements ExecutorBackend for Sprites.dev remote sandboxes.
type SpritesExecutor struct {
	secretStore      secrets.SecretStore
	agentList        RemoteAuthAgentLister
	agentctlResolver *AgentctlResolver
	logger           *logger.Logger
	agentctlPort     int
	mu               sync.RWMutex
	proxies          map[string]*SpritesProxySession
	tokens           map[string]string // cached API tokens per instance
}

// NewSpritesExecutor creates a new Sprites runtime.
func NewSpritesExecutor(
	secretStore secrets.SecretStore,
	agentList RemoteAuthAgentLister,
	resolver *AgentctlResolver,
	agentctlPort int,
	log *logger.Logger,
) *SpritesExecutor {
	return &SpritesExecutor{
		secretStore:      secretStore,
		agentList:        agentList,
		agentctlResolver: resolver,
		logger:           log.WithFields(zap.String("runtime", "sprites")),
		agentctlPort:     agentctlPort,
		proxies:          make(map[string]*SpritesProxySession),
		tokens:           make(map[string]string),
	}
}

func (r *SpritesExecutor) Name() executor.Name {
	return executor.NameSprites
}

func (r *SpritesExecutor) HealthCheck(_ context.Context) error {
	return nil
}

func (r *SpritesExecutor) ResumeRemoteInstance(_ context.Context, req *ExecutorCreateRequest) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	return nil
}

func (r *SpritesExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	token := req.Env["SPRITES_API_TOKEN"]
	if token == "" {
		return nil, fmt.Errorf("SPRITES_API_TOKEN not set in execution environment (configure it in the executor profile)")
	}

	r.mu.Lock()
	r.tokens[req.InstanceID] = token
	r.mu.Unlock()

	reconnect := req.PreviousExecutionID != ""
	spriteName := r.resolveSpriteName(req, reconnect)
	client := sprites.New(token, sprites.WithDisableControl())
	report := newStepReporter(req.OnProgress)
	destroyOnFailure := !reconnect

	r.logger.Info("creating sprite instance",
		zap.String("instance_id", req.InstanceID),
		zap.String("sprite_name", spriteName),
		zap.Bool("reconnect_required", reconnect))

	// Step 0: Create or reconnect sprite
	sprite, err := r.stepCreateSprite(ctx, client, spriteName, reconnect, report)
	if err != nil {
		r.cleanupOnFailure(ctx, sprite, req.InstanceID, destroyOnFailure)
		return nil, err
	}

	// Steps 1-3: Upload agentctl, credentials, prepare script
	if err := r.stepSetupEnvironment(ctx, sprite, req, reconnect, report); err != nil {
		r.cleanupOnFailure(ctx, sprite, req.InstanceID, destroyOnFailure)
		return nil, err
	}

	// Step 4: Wait for agentctl health
	if err := r.stepWaitHealthy(ctx, sprite, report); err != nil {
		r.cleanupOnFailure(ctx, sprite, req.InstanceID, destroyOnFailure)
		return nil, err
	}

	// Step 5: Create or reuse agent instance
	instancePort, reusingExisting, err := r.stepEnsureAgentInstance(ctx, sprite, req, reconnect, report)
	if err != nil {
		r.cleanupOnFailure(ctx, sprite, req.InstanceID, destroyOnFailure)
		return nil, err
	}

	// Step 6: Network policy
	r.stepApplyNetworkPolicy(ctx, client, spriteName, req, report)

	// Port forwarding to the per-instance server
	localPort, err := r.setupPortForwarding(ctx, sprite, spriteName, req.InstanceID, instancePort)
	if err != nil {
		r.cleanupOnFailure(ctx, sprite, req.InstanceID, destroyOnFailure)
		return nil, err
	}

	r.logger.Info("sprite instance ready",
		zap.String("instance_id", req.InstanceID),
		zap.String("sprite_name", spriteName),
		zap.Int("local_port", localPort),
		zap.Int("instance_port", instancePort))

	return r.buildInstanceResult(req, spriteName, sprite, localPort, instancePort, reusingExisting), nil
}

// resolveSpriteName determines the sprite name based on request and reconnect state.
func (r *SpritesExecutor) resolveSpriteName(req *ExecutorCreateRequest, reconnect bool) string {
	if !reconnect {
		return spritesNamePrefix + req.InstanceID[:12]
	}
	if name := getMetadataString(req.Metadata, "sprite_name"); name != "" {
		return name
	}
	suffix := req.PreviousExecutionID
	if len(suffix) > 12 {
		suffix = suffix[:12]
	}
	return spritesNamePrefix + suffix
}

// stepCreateSprite handles step 0: create or reconnect a sprite.
func (r *SpritesExecutor) stepCreateSprite(
	ctx context.Context,
	client *sprites.Client,
	name string,
	reconnect bool,
	report func(PrepareStep, int),
) (*sprites.Sprite, error) {
	stepName := "Creating cloud sandbox"
	if reconnect {
		stepName = "Reconnecting cloud sandbox"
	}
	step := beginStep(stepName)
	report(step, 0)

	var sprite *sprites.Sprite
	var err error
	if reconnect {
		sprite, err = r.reconnectSprite(ctx, client, name)
	} else {
		sprite, err = r.createSprite(ctx, client, name)
	}
	if err != nil {
		completeStepError(&step, err.Error())
		report(step, 0)
		return sprite, err
	}
	completeStepSuccess(&step)
	report(step, 0)
	return sprite, nil
}

// stepSetupEnvironment handles steps 1-3: upload agentctl, credentials, and prepare script.
// All steps are skipped when reconnecting to an existing sprite.
func (r *SpritesExecutor) stepSetupEnvironment(
	ctx context.Context,
	sprite *sprites.Sprite,
	req *ExecutorCreateRequest,
	reconnect bool,
	report func(PrepareStep, int),
) error {
	// Step 1: Upload agentctl binary
	step := beginStep("Uploading agent controller")
	report(step, 1)
	if reconnect {
		completeStepSkipped(&step)
	} else {
		if err := r.uploadAgentctl(ctx, sprite); err != nil {
			completeStepError(&step, err.Error())
			report(step, 1)
			return err
		}
		completeStepSuccess(&step)
	}
	report(step, 1)

	// Step 2: Upload remote credentials (SSH keys, gh CLI, agent auth)
	step = beginStep("Uploading credentials")
	report(step, 2)
	if reconnect {
		completeStepSkipped(&step)
	} else {
		if err := r.uploadCredentials(ctx, sprite, req, func(output string) {
			step.Output = output
			report(step, 2)
		}); err != nil {
			r.logger.Warn("failed to upload credentials (non-fatal)", zap.Error(err))
			completeStepSkipped(&step)
		} else {
			completeStepSuccess(&step)
		}
	}
	report(step, 2)

	// Step 3: Run prepare script (with output streaming)
	step = beginStep("Running prepare script")
	report(step, 3)
	if reconnect {
		completeStepSkipped(&step)
		report(step, 3)
		return nil
	}
	err := r.runPrepareScript(ctx, sprite, req, func(output string) {
		step.Output = output
		report(step, 3)
	})
	if err != nil {
		completeStepError(&step, err.Error())
		report(step, 3)
		return err
	}
	completeStepSuccess(&step)
	report(step, 3)
	return nil
}

// stepWaitHealthy handles step 4: wait for agentctl health inside the sprite.
func (r *SpritesExecutor) stepWaitHealthy(
	ctx context.Context,
	sprite *sprites.Sprite,
	report func(PrepareStep, int),
) error {
	step := beginStep("Waiting for agent controller")
	report(step, 4)
	if err := r.waitForHealth(ctx, sprite); err != nil {
		completeStepError(&step, err.Error())
		report(step, 4)
		return err
	}
	completeStepSuccess(&step)
	report(step, 4)
	return nil
}

// stepEnsureAgentInstance handles step 5: create or reuse an agent instance.
// On reconnect, checks if the existing instance and its subprocess are still running.
func (r *SpritesExecutor) stepEnsureAgentInstance(
	ctx context.Context,
	sprite *sprites.Sprite,
	req *ExecutorCreateRequest,
	reconnect bool,
	report func(PrepareStep, int),
) (int, bool, error) {
	step := beginStep("Creating agent instance")
	report(step, 5)

	var instancePort int
	var reusingExisting bool
	if reconnect {
		port, portErr := r.getExistingInstancePort(ctx, sprite, req.PreviousExecutionID)
		if portErr == nil && port > 0 && r.isAgentSubprocessRunning(ctx, sprite, port) {
			instancePort = port
			reusingExisting = true
		} else if portErr == nil && port > 0 {
			r.logger.Info("existing instance found but agent subprocess not running, creating fresh instance",
				zap.String("instance_id", req.PreviousExecutionID),
				zap.Int("port", port))
		}
	}
	if instancePort == 0 {
		port, err := r.createAgentInstance(ctx, sprite, req)
		if err != nil {
			completeStepError(&step, err.Error())
			report(step, 5)
			return 0, false, err
		}
		instancePort = port
	}
	completeStepSuccess(&step)
	report(step, 5)
	return instancePort, reusingExisting, nil
}

// stepApplyNetworkPolicy handles step 6: apply network policy from the executor profile.
func (r *SpritesExecutor) stepApplyNetworkPolicy(
	ctx context.Context,
	client *sprites.Client,
	spriteName string,
	req *ExecutorCreateRequest,
	report func(PrepareStep, int),
) {
	step := beginStep("Applying network policy")
	report(step, 6)
	if err := r.applyNetworkPolicy(ctx, client, spriteName, req); err != nil {
		r.logger.Warn("failed to apply network policy from profile", zap.Error(err))
		completeStepSkipped(&step)
	} else {
		completeStepSuccess(&step)
	}
	report(step, 6)
}

// buildInstanceResult constructs the final ExecutorInstance from sprite creation results.
// Note: Sprites do not use agentctl auth tokens because each sprite is an isolated VM,
// and reconnect scenarios cannot retrieve the original token from the running agentctl.
func (r *SpritesExecutor) buildInstanceResult(
	req *ExecutorCreateRequest,
	spriteName string,
	sprite *sprites.Sprite,
	localPort, instancePort int,
	reusingExisting bool,
) *ExecutorInstance {
	return &ExecutorInstance{
		InstanceID:  req.InstanceID,
		TaskID:      req.TaskID,
		SessionID:   req.SessionID,
		RuntimeName: string(r.Name()),
		Client: agentctl.NewClient("127.0.0.1", localPort, r.logger,
			agentctl.WithExecutionID(req.InstanceID),
			agentctl.WithSessionID(req.SessionID)),
		WorkspacePath: spritesWorkspacePath,
		Metadata: map[string]interface{}{
			"sprite_name":            spriteName,
			"sprite_state":           strings.TrimSpace(sprite.Status),
			"sprite_created_at":      sprite.CreatedAt,
			"local_port":             localPort,
			"reuse_existing_process": reusingExisting,
			MetadataKeyIsRemote:      true,
		},
	}
}

func (r *SpritesExecutor) RecoverInstances(_ context.Context) ([]*ExecutorInstance, error) {
	return nil, nil
}

func (r *SpritesExecutor) GetInteractiveRunner() *process.InteractiveRunner {
	return nil
}

func (r *SpritesExecutor) RequiresCloneURL() bool          { return true }
func (r *SpritesExecutor) ShouldApplyPreferredShell() bool { return false }
func (r *SpritesExecutor) IsAlwaysResumable() bool         { return true }
