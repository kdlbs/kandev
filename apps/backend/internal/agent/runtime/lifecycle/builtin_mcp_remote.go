package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

func requireBuiltinRemoteHelper(ctx context.Context, req *ExecutorCreateRequest, fetch func(context.Context) ([]byte, error)) error {
	if req.McpProfile == nil || req.McpProfile.BuiltinPackages == nil {
		return nil
	}
	data, err := fetch(ctx)
	if err != nil {
		return fmt.Errorf("check built-in MCP helper support: %w", err)
	}
	var identity agentctl.IdentityInfo
	if err := json.Unmarshal(data, &identity); err != nil {
		return fmt.Errorf("read built-in MCP helper identity: %w", err)
	}
	return agentctl.ValidateBuiltinHelper(req.McpProfile, req.TaskID, &identity)
}

func requireSSHBuiltinHelper(ctx context.Context, client *http.Client, port int, authToken string, req *ExecutorCreateRequest) error {
	return requireBuiltinRemoteHelper(ctx, req, func(ctx context.Context) ([]byte, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+"/identity", nil)
		if err != nil {
			return nil, err
		}
		setSSHControlAuthorization(request, authToken)
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("agentctl identity returned HTTP %d", response.StatusCode)
		}
		return io.ReadAll(io.LimitReader(response.Body, 64*1024))
	})
}
