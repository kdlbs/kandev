// serve.go wires plugin authors (via Serve) and kandev's runtime manager
// (via the exported GRPCPlugin type) onto hashicorp/go-plugin, per §2/§3 of
// docs/plans/plugins/GRPC-CONTRACT.md.
//
// # Host injection
//
// The frozen Plugin service (§3) has no RPC for negotiating a broker ID, so
// the two sides can't use go-plugin's usual "allocate an ID and pass it in
// the request" bidirectional pattern for the Host service. Instead they
// agree on a constant, hostBrokerID:
//
//   - kandev (host side, GRPCPlugin.GRPCClient — called once when kandev
//     dispenses the plugin) starts serving its own Host implementation on
//     hostBrokerID via broker.AcceptAndServe.
//   - The plugin subprocess (GRPCPlugin.GRPCServer — called once at
//     subprocess startup, before the process has even accepted a
//     connection from kandev) cannot dial hostBrokerID synchronously: the
//     broker's control stream to kandev doesn't exist yet, so a blocking
//     Dial here would deadlock startup. It instead spawns a background
//     goroutine that retries broker.Dial(hostBrokerID) with backoff until
//     it succeeds or HostDialTimeout elapses, then injects the resulting
//     Host into Impl via the optional HostSetter interface —
//     UnimplementedPlugin implements HostSetter, so embedding it is enough
//     to opt in. A Plugin that doesn't implement HostSetter simply never
//     receives a Host (fine for plugins that don't need one).
//
// GRPCPlugin is exported specifically so kandev's runtime manager can reuse
// it on the host side with plugin.NewClient — see the GRPCPlugin doc
// comment for the exact wiring.
package pluginsdk

import (
	"context"
	"fmt"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/grpc"
)

// Handshake is the go-plugin handshake shared by kandev and every plugin
// backend (§2 of docs/plans/plugins/GRPC-CONTRACT.md). Both sides must use
// this exact value — Serve uses it automatically; kandev's runtime manager
// must set it on plugin.ClientConfig.HandshakeConfig.
var Handshake = hcplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "KANDEV_PLUGIN",
	MagicCookieValue: "kandev-plugin-v1",
}

// PluginMapKey is the go-plugin plugin-map key both Serve (plugin side) and
// kandev's runtime manager (host side, plugin.ClientConfig.Plugins) use.
const PluginMapKey = "plugin"

// hostBrokerID is the well-known go-plugin broker stream ID used for the
// Host service. See the "Host injection" section in the file header.
const hostBrokerID = 1

// defaultHostDialTimeout bounds how long the plugin subprocess retries
// dialing hostBrokerID before giving up and leaving Host unset.
const defaultHostDialTimeout = 30 * time.Second

// GRPCPlugin adapts a Plugin (subprocess side) and/or a Host (kandev side)
// to hashicorp/go-plugin's plugin.GRPCPlugin interface. It is the shared
// type both sides use:
//
//   - Plugin authors never construct it directly; Serve does so internally.
//
//   - kandev's runtime manager constructs it directly:
//
//     client := plugin.NewClient(&plugin.ClientConfig{
//     HandshakeConfig:  pluginsdk.Handshake,
//     Plugins:          map[string]plugin.Plugin{pluginsdk.PluginMapKey: &pluginsdk.GRPCPlugin{Host: myHostImpl}},
//     AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
//     AutoMTLS:         true,
//     Cmd:              exec.Command(pluginBinaryPath),
//     })
//     rpcClient, _ := client.Client()
//     raw, _ := rpcClient.Dispense(pluginsdk.PluginMapKey)
//     remote := raw.(*pluginsdk.RemotePlugin)
//
// See the file header for how Host injection works across the two sides.
type GRPCPlugin struct {
	hcplugin.NetRPCUnsupportedPlugin

	// Impl is the plugin author's implementation. Set by Serve on the
	// plugin subprocess side.
	Impl Plugin

	// Host is kandev's Go-native Host implementation. Set by kandev's
	// runtime manager on the host side.
	Host Host

	// HostDialTimeout overrides defaultHostDialTimeout. Zero means use the
	// default; mainly useful in tests.
	HostDialTimeout time.Duration
}

var _ hcplugin.GRPCPlugin = (*GRPCPlugin)(nil)

// GRPCServer is called once inside the plugin subprocess (by plugin.Serve,
// via Serve) to register the Plugin service and — if Impl implements
// HostSetter — kick off the background Host dial described in the file
// header.
func (p *GRPCPlugin) GRPCServer(broker *hcplugin.GRPCBroker, s *grpc.Server) error {
	pluginv1.RegisterPluginServer(s, &grpcPluginServer{impl: p.Impl})

	setter, ok := p.Impl.(HostSetter)
	if !ok {
		return nil
	}
	timeout := p.HostDialTimeout
	if timeout <= 0 {
		timeout = defaultHostDialTimeout
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		conn, err := dialBrokerWithRetry(ctx, broker, hostBrokerID)
		if err != nil {
			return
		}
		setter.SetHost(newHostClient(conn))
	}()
	return nil
}

// GRPCClient is called once inside kandev (by plugin.NewClient's Dispense)
// to build the RemotePlugin used to call the plugin, and — if Host is set —
// to start serving it on hostBrokerID so the plugin subprocess can call
// back. See the file header.
func (p *GRPCPlugin) GRPCClient(_ context.Context, broker *hcplugin.GRPCBroker, conn *grpc.ClientConn) (interface{}, error) {
	if p.Host != nil {
		go broker.AcceptAndServe(hostBrokerID, func(opts []grpc.ServerOption) *grpc.Server {
			s := grpc.NewServer(opts...)
			registerHostServer(s, p.Host)
			return s
		})
	}
	return &RemotePlugin{client: pluginv1.NewPluginClient(conn)}, nil
}

// dialBrokerWithRetry retries broker.Dial(id) with backoff until it
// succeeds or ctx is done. A single Dial attempt can legitimately fail
// while the peer hasn't called AcceptAndServe(id, ...) yet (see the file
// header); retrying absorbs that startup race.
func dialBrokerWithRetry(ctx context.Context, broker *hcplugin.GRPCBroker, id uint32) (*grpc.ClientConn, error) {
	backoff := 100 * time.Millisecond
	const maxBackoff = 2 * time.Second
	for {
		conn, err := broker.Dial(id)
		if err == nil {
			return conn, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("pluginsdk: dial broker id %d: %w", id, ctx.Err())
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
		}
	}
}

// RemotePlugin is the Go-native client kandev's runtime manager uses to
// call a running plugin, returned by Dispense(pluginsdk.PluginMapKey).
type RemotePlugin struct {
	client pluginv1.PluginClient
}

// DeliverEvent calls the plugin's DeliverEvent RPC. Timeout/retry policy
// (§5: 10s timeout, 3 retries) is the caller's responsibility — this method
// is a thin, context-respecting proxy.
func (r *RemotePlugin) DeliverEvent(ctx context.Context, e *Event) error {
	proto, err := e.toProto()
	if err != nil {
		return err
	}
	_, err = r.client.DeliverEvent(ctx, proto)
	return err
}

// HandleWebhook calls the plugin's HandleWebhook RPC.
func (r *RemotePlugin) HandleWebhook(ctx context.Context, req *WebhookRequest) (*WebhookResponse, error) {
	resp, err := r.client.HandleWebhook(ctx, req.toProto())
	if err != nil {
		return nil, err
	}
	return webhookResponseFromProto(resp), nil
}

// grpcPluginServer adapts the author-facing Plugin interface to the
// generated pluginv1.PluginServer interface. Registered inside the plugin
// subprocess by GRPCPlugin.GRPCServer.
type grpcPluginServer struct {
	pluginv1.UnimplementedPluginServer
	impl Plugin
}

func (s *grpcPluginServer) DeliverEvent(ctx context.Context, req *pluginv1.Event) (*pluginv1.EventAck, error) {
	e, err := eventFromProto(req)
	if err != nil {
		return nil, err
	}
	if err := s.impl.OnEvent(ctx, e); err != nil {
		return nil, err
	}
	return &pluginv1.EventAck{}, nil
}

func (s *grpcPluginServer) HandleWebhook(ctx context.Context, req *pluginv1.WebhookRequest) (*pluginv1.WebhookResponse, error) {
	resp, err := s.impl.HandleWebhook(ctx, webhookRequestFromProto(req))
	if err != nil {
		return nil, err
	}
	return resp.toProto(), nil
}

var _ pluginv1.PluginServer = (*grpcPluginServer)(nil)

// Option configures Serve.
type Option func(*serveConfig)

type serveConfig struct {
	hostDialTimeout time.Duration
}

// WithHostDialTimeout overrides how long Serve retries dialing kandev's
// Host broker connection before giving up (default 30s). Mainly useful in
// tests that want a tighter bound than production.
func WithHostDialTimeout(d time.Duration) Option {
	return func(c *serveConfig) { c.hostDialTimeout = d }
}

// Serve wires p up as a kandev plugin backend and blocks until the process
// is terminated by kandev. It owns all go-plugin/grpc plumbing: the
// handshake (Handshake), the plugin map (PluginMapKey), and the Host broker
// dial + injection described in the file header.
func Serve(p Plugin, opts ...Option) {
	cfg := &serveConfig{hostDialTimeout: defaultHostDialTimeout}
	for _, opt := range opts {
		opt(cfg)
	}
	gp := &GRPCPlugin{Impl: p, HostDialTimeout: cfg.hostDialTimeout}
	hcplugin.Serve(&hcplugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins:         map[string]hcplugin.Plugin{PluginMapKey: gp},
		GRPCServer:      hcplugin.DefaultGRPCServer,
	})
}
