package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	sprites "github.com/superfly/sprites-go"
	"go.uber.org/zap"
)

func (r *SpritesExecutor) setupPortForwarding(
	_ context.Context,
	sprite *sprites.Sprite,
	spriteName, instanceID string,
	remotePort int,
) (int, error) {
	localPort, err := getFreePort()
	if err != nil {
		return 0, fmt.Errorf("failed to get free port: %w", err)
	}

	r.logger.Debug("setting up port forwarding",
		zap.Int(MetadataKeyLocalPort, localPort),
		zap.Int("remote_port", remotePort))

	proxyCtx, cancel := context.WithCancel(context.Background())
	session, err := sprite.ProxyPort(proxyCtx, localPort, remotePort)
	if err != nil {
		cancel()
		return 0, fmt.Errorf("port forwarding failed: %w", err)
	}

	r.mu.Lock()
	r.proxies[instanceID] = &SpritesProxySession{
		spriteName:   spriteName,
		localPort:    localPort,
		proxySession: session,
		cancel:       cancel,
	}
	r.mu.Unlock()

	return localPort, nil
}

func (r *SpritesExecutor) applyNetworkPolicy(
	ctx context.Context,
	client *sprites.Client,
	spriteName string,
	req *ExecutorCreateRequest,
) error {
	rulesJSON, _ := req.Metadata["sprites_network_policy_rules"].(string)
	if rulesJSON == "" {
		return nil
	}

	var rules []sprites.NetworkPolicyRule
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return fmt.Errorf("failed to parse network policy rules: %w", err)
	}
	if len(rules) == 0 {
		return nil
	}

	policyCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	r.logger.Info("applying network policy from profile",
		zap.String(MetadataKeySpriteName, spriteName),
		zap.Int("rule_count", len(rules)))

	return client.UpdateNetworkPolicy(policyCtx, spriteName, &sprites.NetworkPolicy{Rules: rules})
}

// getFreePort finds an available local port.
func getFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}
