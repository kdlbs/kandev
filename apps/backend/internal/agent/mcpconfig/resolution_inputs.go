package mcpconfig

import (
	"context"
	"errors"
)

func (r *Resolver) readResolutionInputs(ctx context.Context, rc ResolutionContext) (*ResolutionInputs, error) {
	if reader, ok := r.selections.(ResolutionInputsRepository); ok {
		return reader.ReadMCPResolutionInputs(ctx, rc)
	}
	origins, _, err := r.loadOrigins(ctx, rc)
	if err != nil {
		return nil, err
	}
	inputs := &ResolutionInputs{Origins: origins, Definitions: make(map[string]*MCPServerDefinition, len(origins))}
	for id := range origins {
		d, err := r.catalog.GetMCPServerDefinition(ctx, rc.WorkspaceID, id)
		if err != nil && !errors.Is(err, ErrMCPServerDefinitionNotFound) {
			return nil, err
		}
		inputs.Definitions[id] = d
	}
	return inputs, nil
}
