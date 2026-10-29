package controller

import (
	"github.com/kandev/kandev/internal/agent/settings/cliflags"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

func safeCommandPreview(argv []string, prefix string, native bool) (*dto.CommandPreviewResponse, error) {
	tokens, err := cliflags.Tokenise(prefix)
	if err != nil {
		return nil, ErrInvalidCommandPrefix
	}
	wrapper, err := launchwrapper.Profile(tokens).Summary()
	if err != nil {
		return nil, ErrInvalidCommandPrefix
	}
	destination := dto.FlagDestinationACPBridge
	if native {
		destination = dto.FlagDestinationAgentCLI
	}
	return &dto.CommandPreviewResponse{Supported: true, FlagDestination: destination,
		Composition: &dto.CommandComposition{Wrapper: wrapper, Agent: launchwrapper.SummarizeCommand(argv)}}, nil
}
