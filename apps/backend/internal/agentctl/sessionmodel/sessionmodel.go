// Package sessionmodel centralizes ACP model selection across long-lived
// sessions and sessionless utility prompts.
package sessionmodel

import (
	"context"
	"errors"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const modelConfigOption = "model"

// Method is the ACP mechanism used to apply a requested model.
type Method string

const (
	MethodNone            Method = ""
	MethodSetConfigOption Method = "session/set_config_option"
	// MethodSetModel is the pre-v0.13.5 unstable session/set_model RPC.
	// Used as a fallback for agents (e.g. auggie 0.29.x) that surface their
	// model list via the legacy top-level `models` field on session/new
	// instead of SessionConfigOption(category="model"). The RPC is provided
	// by the kdlbs acp-go-sdk fork (ClientSideConnection.UnstableSetSessionModel).
	MethodSetModel Method = "session/set_model"
)

// ConfigOption is the subset of ACP session config options needed to decide
// how a model should be applied.
type ConfigOption struct {
	ID       string
	Category string
}

// Request describes a requested model change.
type Request struct {
	SessionID     string
	ModelID       string
	ConfigOptions []ConfigOption
}

// Applier performs the actual ACP call. Implementations wrap either the ACP
// SDK connection or the agentctl websocket client.
type Applier interface {
	SetConfigOption(ctx context.Context, sessionID, configID, value string) error
	// SetModelLegacy issues the pre-v0.13.5 unstable session/set_model RPC.
	// Used only when the session advertises no model-shaped config option;
	// implementations should return a JSON-RPC -32601 error (recognized by
	// IsMethodNotFound) when the agent doesn't implement the legacy surface,
	// so Apply can treat the model change as a clean no-op.
	SetModelLegacy(ctx context.Context, sessionID, modelID string) error
}

// SDKConn is the subset of the ACP SDK connection used to apply model changes.
type SDKConn interface {
	SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error)
	// UnstableSetSessionModel is the legacy session/set_model RPC restored by
	// the kdlbs acp-go-sdk fork for compatibility with unmigrated agents.
	UnstableSetSessionModel(context.Context, acp.UnstableSetSessionModelRequest) (acp.UnstableSetSessionModelResponse, error)
}

// SDKApplier applies model changes through a typed ACP SDK connection.
type SDKApplier struct {
	Conn          SDKConn
	configOptions *[]acp.SessionConfigOption
}

func (a SDKApplier) SetConfigOption(ctx context.Context, sessionID, configID, value string) error {
	resp, err := a.Conn.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: acp.SessionId(sessionID),
			ConfigId:  acp.SessionConfigId(configID),
			Value:     acp.SessionConfigValueId(value),
		},
	})
	if err == nil && a.configOptions != nil {
		*a.configOptions = resp.ConfigOptions
	}
	return err
}

func (a SDKApplier) SetModelLegacy(ctx context.Context, sessionID, modelID string) error {
	_, err := a.Conn.UnstableSetSessionModel(ctx, acp.UnstableSetSessionModelRequest{
		SessionId: acp.SessionId(sessionID),
		ModelId:   modelID,
	})
	return err
}

// ApplySDK applies a model change through the ACP SDK connection.
func ApplySDK(ctx context.Context, conn SDKConn, req Request) (Method, error) {
	method, _, err := ApplySDKWithConfigOptions(ctx, conn, req)
	return method, err
}

// ApplySDKWithConfigOptions applies a model change and returns the complete
// config option state included in a stable ACP set_config_option response.
func ApplySDKWithConfigOptions(
	ctx context.Context,
	conn SDKConn,
	req Request,
) (Method, []acp.SessionConfigOption, error) {
	var configOptions []acp.SessionConfigOption
	method, err := Apply(ctx, SDKApplier{Conn: conn, configOptions: &configOptions}, req)
	return method, configOptions, err
}

// ApplySDKFromACP applies a model change using typed session config options.
func ApplySDKFromACP(
	ctx context.Context,
	conn SDKConn,
	sessionID string,
	modelID string,
	configOptions []acp.SessionConfigOption,
) (Method, error) {
	return ApplySDK(ctx, conn, Request{
		SessionID:     sessionID,
		ModelID:       modelID,
		ConfigOptions: FromACP(configOptions),
	})
}

// Apply selects the model by routing through the typed
// session/set_config_option RPC when the session advertises a model-shaped
// config option. When that option is absent — or when the modern RPC returns
// JSON-RPC -32601 because a partially-migrated agent advertises the option
// without implementing the handler — falls through to the legacy
// session/set_model RPC. A -32601 from the legacy call is treated as the
// agent declaring "I don't support model selection", and Apply returns
// MethodNone with no error so callers can no-op cleanly.
func Apply(ctx context.Context, applier Applier, req Request) (Method, error) {
	if req.ModelID == "" {
		return MethodNone, nil
	}
	if configID, ok := modelConfigID(req.ConfigOptions); ok {
		err := applier.SetConfigOption(ctx, req.SessionID, configID, req.ModelID)
		if err == nil {
			return MethodSetConfigOption, nil
		}
		if !IsMethodNotFound(err) {
			return MethodSetConfigOption, err
		}
		// Agent advertises the typed option but its handler isn't wired up
		// (partial migration); fall through to the legacy RPC below.
	}
	err := applier.SetModelLegacy(ctx, req.SessionID, req.ModelID)
	if err == nil {
		return MethodSetModel, nil
	}
	if IsMethodNotFound(err) {
		return MethodNone, nil
	}
	return MethodSetModel, err
}

// FromACP converts typed ACP SDK options to the shared strategy shape.
func FromACP(opts []acp.SessionConfigOption) []ConfigOption {
	out := make([]ConfigOption, 0, len(opts))
	for _, opt := range opts {
		if opt.Select == nil {
			continue
		}
		co := ConfigOption{ID: string(opt.Select.Id)}
		if opt.Select.Category != nil {
			co.Category = string(*opt.Select.Category)
		}
		out = append(out, co)
	}
	return out
}

// FromStreams converts normalized stream config options to the shared strategy shape.
func FromStreams(opts []streams.ConfigOption) []ConfigOption {
	out := make([]ConfigOption, 0, len(opts))
	for _, opt := range opts {
		out = append(out, ConfigOption{ID: opt.ID, Category: opt.Category})
	}
	return out
}

func modelConfigID(opts []ConfigOption) (string, bool) {
	for _, opt := range opts {
		if opt.ID != modelConfigOption && opt.Category != modelConfigOption {
			continue
		}
		if opt.ID != "" {
			return opt.ID, true
		}
		return modelConfigOption, true
	}
	return "", false
}

// IsMethodNotFound reports JSON-RPC -32601 failures, even when wrapped by a caller.
func IsMethodNotFound(err error) bool {
	var reqErr *acp.RequestError
	return errors.As(err, &reqErr) && reqErr.Code == -32601
}
