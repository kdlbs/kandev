package plugins

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/kandev/kandev/internal/plugins/executortunnel"
	"github.com/kandev/kandev/internal/plugins/hostcaps"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/protobuf/proto"
)

const (
	executorLeaseTransportHTTPS      = "https"
	executorLeaseTransportHostTunnel = "host_tunnel"
)

// ExecutorTunnelRequest names the environment port a host-opened tunnel reaches.
type ExecutorTunnelRequest struct {
	Context  *pluginsdk.ExecutorProviderRequestContext
	Resource *pluginsdk.ExecutorResourceDescriptor
	Port     uint32
	// AllowedPorts are the agentctl control port and the recorded instance port.
	AllowedPorts []uint32
	// Profile is the recorded profile snapshot with secrets revealed for this stream.
	Profile *pluginsdk.ExecutorProfileSnapshot
}

// openExecutorTunnel opens a host-initiated stream to one port of an
// environment. The stream is bound to the installation, environment and
// ownership generation of the request context, and no request byte flows until
// the provider acknowledges the binding. Plugin restart, disable, uninstall,
// environment release and a newer ownership generation close it.
func (s *Service) openExecutorTunnel(ctx context.Context, request ExecutorTunnelRequest) (net.Conn, error) {
	requestContext := request.Context
	if requestContext == nil || requestContext.GetEnvironmentId() == "" {
		return nil, fmt.Errorf("%w: tunnel identity is incomplete", ErrExecutorProviderUnavailable)
	}
	record, err := s.Get(requestContext.GetPluginId())
	if err != nil || record == nil || requestContext.GetInstallationId() != record.InstallationID {
		return nil, fmt.Errorf("%w: tunnel identity is stale", ErrExecutorProviderUnavailable)
	}
	opener := func(streamCtx context.Context) (executortunnel.Stream, error) {
		return s.openProviderTunnelStream(streamCtx, requestContext)
	}
	trusted := proto.Clone(requestContext).(*pluginsdk.ExecutorProviderRequestContext)
	trusted.OperationId = ""
	return s.tunnels.Open(ctx, executortunnel.OpenRequest{
		Key: executortunnel.Key{
			InstallationID: record.InstallationID, EnvironmentID: requestContext.GetEnvironmentId(),
			Generation: requestContext.GetEnvironmentGeneration(),
		},
		Port: request.Port, AllowedPorts: request.AllowedPorts,
		Header: &pluginsdk.ExecutorTunnelHeader{Context: trusted, Resource: request.Resource, Profile: request.Profile},
		Open:   opener,
	})
}

// openProviderTunnelStream holds the provider dispatch lease for as long as the
// stream lives, so a lifecycle change that cancels admitted calls closes it.
func (s *Service) openProviderTunnelStream(streamCtx context.Context, requestContext *pluginsdk.ExecutorProviderRequestContext) (executortunnel.Stream, error) {
	type opened struct {
		stream executortunnel.Stream
		err    error
	}
	result := make(chan opened, 1)
	go func() {
		err := s.invokeExecutorProvider(streamCtx, requestContext, func(callCtx context.Context, remote *pluginsdk.RemotePlugin, _ *pluginsdk.ExecutorProviderRequestContext) error {
			if err := s.requireTunnelProvider(requestContext); err != nil {
				return err
			}
			stream, err := remote.OpenExecutorTunnel(callCtx)
			result <- opened{stream: stream, err: err}
			if err != nil {
				return err
			}
			<-callCtx.Done()
			return nil
		})
		if err != nil {
			select {
			case result <- opened{err: err}:
			default:
			}
		}
	}()
	select {
	case r := <-result:
		if r.err != nil {
			return nil, r.err
		}
		return r.stream, nil
	case <-streamCtx.Done():
		return nil, streamCtx.Err()
	}
}

func (s *Service) requireTunnelProvider(requestContext *pluginsdk.ExecutorProviderRequestContext) error {
	record, err := s.Get(requestContext.GetPluginId())
	if err != nil {
		return err
	}
	declaration, err := validateExecutorProviderAdmission(record, requestContext.GetProviderKey())
	if err != nil {
		return err
	}
	if !declaration.RequiresHostCapability(hostcaps.TunnelTransport) {
		return fmt.Errorf("%w: provider does not require the tunnel_transport capability", models.ErrExecutorCapabilityMismatch)
	}
	return nil
}

func (s *Service) checkLeaseTransport(requestContext *pluginsdk.ExecutorProviderRequestContext, response *pluginsdk.ResolveExecutorConnectionResponse) error {
	record, err := s.Get(requestContext.GetPluginId())
	if err != nil {
		return err
	}
	declaration, err := validateExecutorProviderAdmission(record, requestContext.GetProviderKey())
	if err != nil {
		return err
	}
	return validateExecutorLeaseTransport(declaration, response.GetLease())
}

// OpenExecutorTunnel is the lifecycle-facing form of openExecutorTunnel.
func (s *Service) OpenExecutorTunnel(ctx context.Context, requestContext *pluginsdk.ExecutorProviderRequestContext, resource *pluginsdk.ExecutorResourceDescriptor, port uint32, allowedPorts []uint32, profile *pluginsdk.ExecutorProfileSnapshot) (net.Conn, error) {
	return s.openExecutorTunnel(ctx, ExecutorTunnelRequest{Context: requestContext, Resource: resource, Port: port, AllowedPorts: allowedPorts, Profile: profile})
}

// CloseExecutorTunnels closes every tunnel of an environment, as on release.
func (s *Service) CloseExecutorTunnels(installationID, environmentID string) {
	s.tunnels.CloseEnvironment(installationID, environmentID)
}

func (s *Service) closeExecutorTunnelsForPlugin(pluginID string) {
	if record, err := s.Get(pluginID); err == nil && record != nil && record.InstallationID != "" {
		s.tunnels.CloseInstallation(record.InstallationID)
	}
}

// validateExecutorLeaseTransport enforces the lease transport rules: https is
// the default; host_tunnel needs the tunnel_transport capability and carries no
// base_url.
func validateExecutorLeaseTransport(declaration *manifest.ExecutorProvider, lease *pluginsdk.ExecutorConnectionLease) error {
	if lease == nil {
		return nil
	}
	switch lease.GetTransport() {
	case "", executorLeaseTransportHTTPS:
		return nil
	case executorLeaseTransportHostTunnel:
		if !declaration.RequiresHostCapability(hostcaps.TunnelTransport) {
			return fmt.Errorf("%w: host_tunnel lease from a provider without tunnel_transport", models.ErrExecutorConflict)
		}
		if lease.GetBaseUrl() != "" {
			return fmt.Errorf("%w: host_tunnel lease must not carry a base_url", models.ErrExecutorConflict)
		}
		return nil
	default:
		return errors.Join(models.ErrExecutorInvalidConfig, errors.New("unknown lease transport"))
	}
}
