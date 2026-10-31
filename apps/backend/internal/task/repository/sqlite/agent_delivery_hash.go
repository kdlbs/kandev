package sqlite

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"hash"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

const agentDeliveryPayloadHashVersion = 1

const (
	agentDeliveryHashNormalizedJSON byte = 1
	agentDeliveryHashRawBytes       byte = 2
)

func agentDeliveryEventHash(event *models.AgentDeliveryEvent) string {
	if event == nil {
		return ""
	}
	mode, content := normalizeAgentDeliveryPayload(event.Payload)
	return agentDeliveryEventHashNormalized(event, mode, content)
}

// agentDeliveryEventHashWithoutToolOutputBytes is the supported v1 form used by
// readers that predate the legacy compatibility field.
func agentDeliveryEventHashWithoutToolOutputBytes(event *models.AgentDeliveryEvent) string {
	if event == nil {
		return ""
	}
	mode, content := normalizeAgentDeliveryPayloadWithoutToolOutputBytes(event.Payload)
	return agentDeliveryEventHashNormalized(event, mode, content)
}

func agentDeliveryEventHashNormalized(event *models.AgentDeliveryEvent, mode byte, content []byte) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte{agentDeliveryPayloadHashVersion})
	writeAgentDeliveryHashBytes(hash, []byte(event.StreamID))
	writeAgentDeliveryHashUint64(hash, uint64(event.Sequence))
	writeAgentDeliveryHashBytes(hash, []byte(event.SessionID))
	writeAgentDeliveryHashBytes(hash, []byte(event.IncarnationID))
	writeAgentDeliveryHashUint64(hash, uint64(event.HarnessGeneration))
	writeAgentDeliveryHashBytes(hash, []byte(event.SubmissionID))
	writeAgentDeliveryHashBytes(hash, []byte(event.EventType))
	if event.Terminal {
		_, _ = hash.Write([]byte{1})
	} else {
		_, _ = hash.Write([]byte{0})
	}
	_, _ = hash.Write([]byte{mode})
	writeAgentDeliveryHashBytes(hash, content)
	return hex.EncodeToString(hash.Sum(nil))
}

func writeAgentDeliveryHashBytes(hash hash.Hash, value []byte) {
	writeAgentDeliveryHashUint64(hash, uint64(len(value)))
	_, _ = hash.Write(value)
}

func writeAgentDeliveryHashUint64(hash hash.Hash, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = hash.Write(encoded[:])
}

func normalizeAgentDeliveryPayload(payload []byte) (byte, []byte) {
	var event streams.AgentEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return agentDeliveryHashRawBytes, payload
	}
	return normalizeDecodedAgentDeliveryPayload(&event, payload)
}

func normalizeAgentDeliveryPayloadWithoutToolOutputBytes(payload []byte) (byte, []byte) {
	var event streams.AgentEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return agentDeliveryHashRawBytes, payload
	}
	event.ToolOutputBytes = nil
	return normalizeDecodedAgentDeliveryPayload(&event, payload)
}

func normalizeDecodedAgentDeliveryPayload(event *streams.AgentEvent, payload []byte) (byte, []byte) {
	stripLifecycleDeliveryFields(event)
	normalized, err := json.Marshal(event)
	if err != nil {
		return agentDeliveryHashRawBytes, payload
	}
	return agentDeliveryHashNormalizedJSON, normalized
}
