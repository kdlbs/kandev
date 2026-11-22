package launchwrapper

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenericWrapperEnvelope(t *testing.T) {
	for _, input := range []string{`null`, `{}`, `{"version":2,"source":"profile","argv":["runner"]}`, `{"version":1,"source":"request","argv":["runner"]}`, `{"version":1,"source":"profile","argv":[]}`, `{"version":1,"source":"profile","argv":["runner"],"authorized":true}`} {
		t.Run(input, func(t *testing.T) {
			var spec Spec
			require.Error(t, json.Unmarshal([]byte(input), &spec))
		})
	}
	var spec Spec
	require.NoError(t, json.Unmarshal([]byte(`{"version":1,"source":"profile","argv":["runner","", "{{kandev.work_dir}}"]}`), &spec))
	require.Equal(t, []string{"runner", "", WorkDir}, spec.Argv)
}
