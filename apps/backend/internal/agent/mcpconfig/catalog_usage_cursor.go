package mcpconfig

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"sync"
)

// Keys are process-local: after a restart callers refresh the usage list. Every
// page reauthorizes its owners; the encrypted position grants no access.
type usageCursorCodec struct {
	once   sync.Once
	cipher cipher.AEAD
	err    error
}

func (c *usageCursorCodec) initialize() error {
	c.once.Do(func() {
		key := make([]byte, 32)
		if _, c.err = rand.Read(key); c.err != nil {
			return
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			c.err = err
			return
		}
		c.cipher, c.err = cipher.NewGCMWithRandomNonce(block)
	})
	return c.err
}
func usageCursorContext(workspaceID, definitionID string) []byte {
	data, _ := json.Marshal([]string{"mcp-usage-v1", workspaceID, definitionID})
	return data
}
func (c *usageCursorCodec) encode(ref UsageReference, workspaceID, definitionID string) (string, error) {
	if err := c.initialize(); err != nil {
		return "", err
	}
	data, err := json.Marshal(ref)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(c.cipher.Seal(nil, nil, data, usageCursorContext(workspaceID, definitionID))), nil
}
func (c *usageCursorCodec) decode(token, workspaceID, definitionID string) (UsageReference, error) {
	var ref UsageReference
	if token == "" {
		return ref, nil
	}
	if len(token) > 4096 {
		return ref, ErrMCPUsageCursor
	}
	if err := c.initialize(); err != nil {
		return ref, err
	}
	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return ref, ErrMCPUsageCursor
	}
	plain, err := c.cipher.Open(nil, nil, data, usageCursorContext(workspaceID, definitionID))
	if err != nil {
		return ref, ErrMCPUsageCursor
	}
	if json.Unmarshal(plain, &ref) != nil || ref.OwnerID == "" {
		return UsageReference{}, ErrMCPUsageCursor
	}
	return ref, nil
}
