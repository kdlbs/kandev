package journal

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
)

func validSubmissionTransition(from, to SubmissionState) bool {
	if from == to {
		return true
	}
	switch from {
	case SubmissionPrepared:
		return to == SubmissionAccepted
	case SubmissionAccepted:
		return to == SubmissionDispatching || to == SubmissionCancelled
	case SubmissionDispatching:
		return to == SubmissionCompleted || to == SubmissionFailed || to == SubmissionCancelled || to == SubmissionInterruptedUnknown
	default:
		return false
	}
}

func SubmissionHash(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func sequenceKey(sequence uint64) []byte {
	var key [8]byte
	binary.BigEndian.PutUint64(key[:], sequence)
	return key[:]
}

func encodeInt64(value int64) []byte {
	var data [8]byte
	binary.BigEndian.PutUint64(data[:], uint64(value))
	return data[:]
}

func decodeInt64(data []byte) (int64, error) {
	if len(data) != 8 {
		return 0, ErrJournalCorrupt
	}
	return int64(binary.BigEndian.Uint64(data)), nil
}

func decodeStream(data []byte) (Stream, error) {
	if len(data) == 0 {
		return Stream{}, nil
	}
	var stream Stream
	if err := json.Unmarshal(data, &stream); err != nil || stream.StreamID == "" {
		return Stream{}, ErrJournalCorrupt
	}
	return stream, nil
}
