package lifecycle

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	sprites "github.com/superfly/sprites-go"
	"go.uber.org/zap"
)

var uploadHTTPStatusRE = regexp.MustCompile(`(?i)\b(?:http|status)\s*:?\s*(\d{3})\b`)

func (r *SpritesExecutor) injectTokenIntoURL(remoteURL string, env map[string]string) string {
	token := env["GITHUB_TOKEN"]
	if token == "" {
		return remoteURL
	}
	if converted := rewriteGitHubSSHToHTTPS(remoteURL); converted != "" {
		remoteURL = converted
	}
	if strings.HasPrefix(remoteURL, "https://") {
		return strings.Replace(remoteURL, "https://", "https://"+token+"@", 1)
	}
	return remoteURL
}

func rewriteGitHubSSHToHTTPS(remoteURL string) string {
	const (
		sshPrefixA = "git@github.com:"
		sshPrefixB = "ssh://git@github.com/"
	)
	switch {
	case strings.HasPrefix(remoteURL, sshPrefixA):
		return "https://github.com/" + strings.TrimPrefix(remoteURL, sshPrefixA)
	case strings.HasPrefix(remoteURL, sshPrefixB):
		return "https://github.com/" + strings.TrimPrefix(remoteURL, sshPrefixB)
	default:
		return ""
	}
}

func (r *SpritesExecutor) writeFileWithRetry(
	ctx context.Context,
	sprite *sprites.Sprite,
	path string,
	data []byte,
	mode os.FileMode,
) error {
	backoff := 700 * time.Millisecond
	var lastErr error
	for attempt := 1; attempt <= spriteUploadMaxRetries+1; attempt++ {
		err := sprite.Filesystem().WriteFileContext(ctx, path, data, mode)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt > spriteUploadMaxRetries || !isTransientUploadError(err) || ctx.Err() != nil {
			break
		}

		jitter := time.Duration(rand.Intn(300)) * time.Millisecond
		wait := backoff + jitter
		r.logger.Warn("retrying sprite file upload after transient error",
			zap.String("path", path),
			zap.Int("attempt", attempt),
			zap.Duration("retry_in", wait),
			zap.Error(err))
		time.Sleep(wait)
		backoff *= 2
	}
	return lastErr
}

func isTransientUploadError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	msg := strings.ToLower(err.Error())
	if status := extractUploadHTTPStatus(msg); status != 0 {
		if status == 408 || status == 429 || status >= 500 {
			return true
		}
	}
	return strings.Contains(msg, "client.timeout exceeded while awaiting headers") ||
		strings.Contains(msg, "request canceled") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "text file busy") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "temporary")
}

func extractUploadHTTPStatus(msg string) int {
	matches := uploadHTTPStatusRE.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return 0
	}
	code, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return code
}

func (r *SpritesExecutor) buildSpriteEnv(env map[string]string) []string {
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}

type spritesStepKey string

const (
	spriteStepCreateSprite       spritesStepKey = "create_sprite"
	spriteStepUploadAgentctl     spritesStepKey = "upload_agentctl"
	spriteStepUploadCredentials  spritesStepKey = "upload_credentials"
	spriteStepRunPrepareScript   spritesStepKey = "run_prepare_script"
	spriteStepWaitHealthy        spritesStepKey = "wait_healthy"
	spriteStepAgentInstance      spritesStepKey = "agent_instance"
	spriteStepApplyNetworkPolicy spritesStepKey = "apply_network_policy"
)

type spritesProgressPlan struct {
	steps   []spritesStepKey
	indexes map[spritesStepKey]int
}

func newSpritesProgressPlan(reconnect bool) spritesProgressPlan {
	steps := []spritesStepKey{spriteStepCreateSprite}
	if !reconnect {
		steps = append(steps,
			spriteStepUploadAgentctl,
			spriteStepUploadCredentials,
			spriteStepRunPrepareScript,
		)
	}
	steps = append(steps, spriteStepWaitHealthy, spriteStepAgentInstance)
	if !reconnect {
		steps = append(steps, spriteStepApplyNetworkPolicy)
	}

	indexes := make(map[spritesStepKey]int, len(steps))
	for i, key := range steps {
		indexes[key] = i
	}
	return spritesProgressPlan{steps: steps, indexes: indexes}
}

func (p spritesProgressPlan) total() int {
	return len(p.steps)
}

func (p spritesProgressPlan) has(key spritesStepKey) bool {
	_, ok := p.indexes[key]
	return ok
}

func (p spritesProgressPlan) index(key spritesStepKey) int {
	idx, ok := p.indexes[key]
	if !ok {
		return -1
	}
	return idx
}

// newSpritesStepReporter creates a reporting function that calls OnProgress if non-nil.
func newSpritesStepReporter(onProgress PrepareProgressCallback, plan spritesProgressPlan) func(spritesStepKey, PrepareStep) {
	return func(key spritesStepKey, step PrepareStep) {
		if onProgress == nil {
			return
		}
		idx := plan.index(key)
		if idx < 0 {
			return
		}
		onProgress(step, idx, plan.total())
	}
}

// lastLines returns the last n lines of s.
func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
