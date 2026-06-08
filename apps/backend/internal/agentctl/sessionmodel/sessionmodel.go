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
	MethodSetModel        Method = "session/set_model"
	MethodSetConfigOption Method = "session/set_config_option"
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

// Applier performs the actual ACP calls. Implementations wrap either the ACP
// SDK connection or the agentctl websocket client.
type Applier interface {
	SetConfigOption(ctx context.Context, sessionID, configID, value string) error
	SetModel(ctx context.Context, sessionID, modelID string) error
}

// SDKConn is the subset of the ACP SDK connection used to apply model changes.
type SDKConn interface {
	SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error)
	UnstableSetSessionModel(context.Context, acp.UnstableSetSessionModelRequest) (acp.UnstableSetSessionModelResponse, error)
}

// SDKApplier applies model changes through a typed ACP SDK connection.
type SDKApplier struct {
	Conn SDKConn
}

func (a SDKApplier) SetConfigOption(ctx context.Context, sessionID, configID, value string) error {
	_, err := a.Conn.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: acp.SessionId(sessionID),
			ConfigId:  acp.SessionConfigId(configID),
			Value:     acp.SessionConfigValueId(value),
		},
	})
	return err
}

func (a SDKApplier) SetModel(ctx context.Context, sessionID, modelID string) error {
	_, err := a.Conn.UnstableSetSessionModel(ctx, acp.UnstableSetSessionModelRequest{
		SessionId: acp.SessionId(sessionID),
		ModelId:   acp.UnstableModelId(modelID),
	})
	return err
}

// ApplySDK applies a model change through the ACP SDK connection.
func ApplySDK(ctx context.Context, conn SDKConn, req Request) (Method, error) {
	return Apply(ctx, SDKApplier{Conn: conn}, req)
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

// Apply chooses the model-switching mechanism supported by the session. Agents
// that expose a model-shaped config option (Codex, Cursor, recent Claude) are
// configured through session/set_config_option. If that RPC is not implemented,
// we fall back to the older unstable session/set_model call.
func Apply(ctx context.Context, applier Applier, req Request) (Method, error) {
	if req.ModelID == "" {
		return MethodNone, nil
	}
	if configID, ok := modelConfigID(req.ConfigOptions); ok {
		if err := applier.SetConfigOption(ctx, req.SessionID, configID, req.ModelID); err != nil {
			if !IsMethodNotFound(err) {
				return MethodSetConfigOption, err
			}
		} else {
			return MethodSetConfigOption, nil
		}
	}
	if err := applier.SetModel(ctx, req.SessionID, req.ModelID); err != nil {
		return MethodSetModel, err
	}
	return MethodSetModel, nil
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
