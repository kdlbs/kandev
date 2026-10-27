package sandboxruntime

import (
	"context"
	"net"
	"net/http"
	"slices"
	"strconv"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/common/sandboxmcp"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func (p *Prepared) prepareMCP(request PrepareRequest, grants contract.Grants) ([]*pluginsdk.SandboxEndpoint, error) {
	p.runtimeNetwork = slices.Clone(grants.Network)
	if !grants.KandevMCP {
		return nil, nil
	}
	if request.MCPForward == nil || request.SessionID == "" {
		return nil, policy.ErrIneligible
	}
	forward := request.MCPForward(p.request.AttemptId, p.request.Generation, p.policy.Digest)
	if forward == nil {
		return nil, policy.ErrIneligible
	}
	relay, err := sandboxmcp.Open(p.manager.ctx, p.guardMCP(forward))
	if err != nil {
		return nil, err
	}
	p.mcp = relay
	host, portText, err := net.SplitHostPort(relay.Address())
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil {
		return nil, err
	}
	p.runtimeNetwork = append(p.runtimeNetwork, contract.NetworkGrant{Host: host, Port: uint16(port), Protocol: "tcp"})
	return []*pluginsdk.SandboxEndpoint{{Key: "kandev.mcp", Host: host, Port: uint32(port), Scheme: "http", Path: "/mcp"}}, nil
}

func (p *Prepared) guardMCP(forward sandboxmcp.Forward) sandboxmcp.Forward {
	return func(ctx context.Context, method string, headers http.Header, body []byte) (*http.Response, error) {
		attempt, err := p.manager.policies.ReadAttempt(ctx, p.orgID, p.request.AttemptId)
		if err != nil {
			return nil, err
		}
		if err = p.manager.VerifyLive(ctx, attempt); err != nil {
			return nil, err
		}
		return forward(ctx, method, headers, body)
	}
}
