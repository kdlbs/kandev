package journal

import (
	"encoding/json"

	bolt "go.etcd.io/bbolt"
)

// A restored successor cannot append to an empty, fully retired predecessor.
// Same-generation native resume and streams with retained output remain writable.
func sealRetiredPredecessorStreamTx(tx *bolt.Tx, submission Submission, recoveryGeneration uint64) error {
	if recoveryGeneration <= submission.HarnessGeneration {
		return nil
	}
	streams := tx.Bucket(bucketStreamMeta)
	stream, err := decodeStream(streams.Get([]byte(submission.StreamID)))
	if err != nil {
		return err
	}
	if stream.StreamID == "" || stream.Sealed || stream.HighWater != stream.Acknowledged || stream.Bytes != 0 {
		return nil
	}
	busy, err := streamHasUnsettledSubmissions(tx, stream)
	if err != nil || busy {
		return err
	}
	stream.Sealed = true
	encoded, err := json.Marshal(stream)
	if err != nil {
		return err
	}
	return streams.Put([]byte(stream.StreamID), encoded)
}
