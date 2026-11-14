package pluginsdk

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageObservationSegmentIdentityRoundTripAndDigest(t *testing.T) {
	var first, second UsageObservationV1
	require.NoError(t, json.Unmarshal([]byte(`{"SubjectBinding":"binding","ItemID":"item","SourceRecordID":"record","Revision":1,"Segment":{"DecompositionID":"source-model-partition-v1","SegmentID":"stable-record-a"}}`), &first))
	require.NoError(t, json.Unmarshal([]byte(`{"SubjectBinding":"binding","ItemID":"item","SourceRecordID":"record","Revision":1,"Segment":{"DecompositionID":"source-model-partition-v1","SegmentID":"stable-record-b"}}`), &second))
	a, err := first.ComputePayloadDigestV1()
	require.NoError(t, err)
	b, err := second.ComputePayloadDigestV1()
	require.NoError(t, err)
	require.NotEqual(t, a, b, "the segment declaration must be authenticated by the payload digest")
	encoded, err := json.Marshal(usageObservationV1FromProto(first.toProto()))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(encoded, &got))
	require.Equal(t, map[string]any{"DecompositionID": "source-model-partition-v1", "SegmentID": "stable-record-a"}, got["Segment"])
}
