// Package tooltokens estimates the token size of MCP tool definitions.
package tooltokens

import (
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

// Estimator identifies the stable tokenizer and projection used by the MCP
// explorer contract.
const Estimator = "o200k_base:mcp-tool-json-v1"

// EstimateToolJSON estimates one complete compact MCP tool JSON object.
func EstimateToolJSON(compactJSON []byte) (int, error) {
	codec, err := o200kCodec()
	if err != nil {
		return 0, err
	}
	return codec.Count(string(compactJSON))
}

var (
	codecOnce sync.Once
	codec     tokenizer.Codec
	codecErr  error
)

func o200kCodec() (tokenizer.Codec, error) {
	codecOnce.Do(func() {
		codec, codecErr = tokenizer.Get(tokenizer.O200kBase)
	})
	return codec, codecErr
}
