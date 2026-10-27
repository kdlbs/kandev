package acp

import (
	"errors"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func newMockACPDialect() acpDialect {
	return acpDialect{responseAttemptReset: mockResponseAttemptResetMeta,
		continuationSupport:         streams.ContinuationNativeSavedHistoryV2,
		continuationSupportForError: mockContinuationSupportForError,
		capacityContinuationSupport: streams.CapacityContinuationMockLiveSessionV1,
		continuationError:           mockContinuationError,
		retainedApplicationErr:      mockRetainedApplicationError}
}

func mockContinuationSupportForError(err error) streams.ContinuationSupport {
	var request *sdk.RequestError
	if !errors.As(err, &request) || request.Code != -32603 {
		return ""
	}
	data, ok := request.Data.(map[string]any)
	if !ok {
		return ""
	}
	meta, ok := nestedMap(data, "kandevMock")
	if !ok || meta["agentManagedPendingTools"] != true {
		return ""
	}
	return streams.ContinuationAgentManagedToolsV3
}

func mockRetainedApplicationError(err error) bool {
	var request *sdk.RequestError
	if !errors.As(err, &request) || request.Code != -32603 {
		return false
	}
	data, ok := request.Data.(map[string]any)
	if !ok {
		return false
	}
	meta, ok := nestedMap(data, "kandevMock")
	return ok && meta["retainedProviderCapacity"] == true
}

func mockContinuationError(err error) bool {
	var request *sdk.RequestError
	if !errors.As(err, &request) || request.Code != -32603 || request.Message != "peer disconnected before response" {
		return false
	}
	data, ok := request.Data.(map[string]any)
	if !ok {
		return false
	}
	meta, ok := nestedMap(data, "kandevMock")
	return ok && meta["continuationInterruption"] == true
}

func mockResponseAttemptResetMeta(meta map[string]any) bool {
	mock, ok := nestedMap(meta, "kandevMock")
	if !ok {
		return false
	}
	reset, ok := mock["responseAttemptReset"].(bool)
	return ok && reset
}
