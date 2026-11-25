package streams

import (
	"encoding/json"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-INCREMENTAL-SHELL-001.1, AC-PLATFORM-INCREMENTAL-SHELL-001.2
func TestShellOutputPatchRoundTrip(t *testing.T) {
	code := 7
	cases := []ShellExecOutput{
		{Stdout: "hello\n", Stderr: "warning", ExitCode: nil},
		{Stdout: "hello\nworld\n", Stderr: "warning", ExitCode: &code},
		{Stdout: "", Stderr: "", ExitCode: nil},
		{Stdout: "\x00\t\r\n雪🌍", StdoutTruncated: true, Truncated: true},
	}
	var base *ShellOutputState
	for _, output := range cases {
		patch, err := EncodeShellOutputPatch(base, output, "epoch", false)
		require.NoError(t, err)
		require.NotNil(t, patch)
		encoded, err := json.Marshal(patch)
		require.NoError(t, err)
		var decoded ShellOutputPatch
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		next, err := ApplyShellOutputPatch(base, &decoded)
		require.NoError(t, err)
		require.Equal(t, output, next.Output)
		base = next
	}
	noChange, err := EncodeShellOutputPatch(base, base.Output, "epoch", false)
	require.NoError(t, err)
	require.Nil(t, noChange)
}

func TestShellOutputPatchSaturatedOverlap(t *testing.T) {
	previous := strings.Repeat("雪", (256*1024)/3)
	base := &ShellOutputState{Epoch: "epoch", Revision: 1, Output: ShellExecOutput{Stdout: previous}}
	next := ShellExecOutput{Stdout: previous[3:] + "a", Truncated: true, StdoutTruncated: true}
	patch, err := EncodeShellOutputPatch(base, next, "epoch", false)
	require.NoError(t, err)
	require.Equal(t, "shift_append", patch.Stdout.Operation)
	require.Equal(t, 3, patch.Stdout.Drop)
	require.Equal(t, []byte("a"), patch.Stdout.Data)
	encoded, err := json.Marshal(patch)
	require.NoError(t, err)
	require.Less(t, len(encoded), 2048)
	result, err := ApplyShellOutputPatch(base, patch)
	require.NoError(t, err)
	require.Equal(t, next, result.Output)
}

func TestShellOutputPatchRejectsWrongBase(t *testing.T) {
	base := &ShellOutputState{Epoch: "epoch", Revision: 3, Output: ShellExecOutput{Stdout: "before"}}
	patch, err := EncodeShellOutputPatch(base, ShellExecOutput{Stdout: "before after"}, "epoch", false)
	require.NoError(t, err)
	for _, wrong := range []*ShellOutputState{nil, {Epoch: "other", Revision: 3, Output: base.Output}, {Epoch: "epoch", Revision: 2, Output: base.Output}, {Epoch: "epoch", Revision: 3, Output: ShellExecOutput{Stdout: "changed"}}} {
		_, err := ApplyShellOutputPatch(wrong, patch)
		require.Error(t, err)
	}
	patch.Stdout.Drop = 1 << 30
	_, err = ApplyShellOutputPatch(base, patch)
	require.Error(t, err)
	require.Equal(t, "before", base.Output.Stdout)
}

func TestShellOutputPatchCheckpointSize(t *testing.T) {
	output := ShellExecOutput{Stdout: strings.Repeat("\x00", 256*1024), Stderr: strings.Repeat("\x01", 256*1024)}
	patch, err := EncodeShellOutputPatch(nil, output, "epoch", true)
	require.NoError(t, err)
	encoded, err := json.Marshal(patch)
	require.NoError(t, err)
	require.Less(t, len(encoded), 1<<20)
	state, err := ApplyShellOutputPatch(nil, patch)
	require.NoError(t, err)
	require.Equal(t, output, state.Output)
}

func TestShellOutputPatchMalformedAndCheckpoints(t *testing.T) {
	base := &ShellOutputState{Epoch: "old", Revision: 7, Output: ShellExecOutput{Stdout: "雪abc"}}
	patch, err := EncodeShellOutputPatch(base, ShellExecOutput{Stdout: "雪abc!"}, "old", false)
	require.NoError(t, err)
	for _, mutate := range []func(*ShellOutputPatch){
		func(p *ShellOutputPatch) { p.EncodingVersion = 2 },
		func(p *ShellOutputPatch) { p.ResultDigest = "changed" },
		func(p *ShellOutputPatch) { p.Stdout.Operation = "unknown" },
		func(p *ShellOutputPatch) { p.Stdout.Drop = 1 },
		func(p *ShellOutputPatch) { p.Stdout.Data = []byte{0xff} },
		func(p *ShellOutputPatch) { p.Stdout.Data = []byte(strings.Repeat("x", MaxShellOutputFieldBytes+1)) },
	} {
		encoded, e := json.Marshal(patch)
		require.NoError(t, e)
		var changed ShellOutputPatch
		require.NoError(t, json.Unmarshal(encoded, &changed))
		mutate(&changed)
		_, e = ApplyShellOutputPatch(base, &changed)
		require.Error(t, e)
	}
	checkpoint, err := EncodeShellOutputPatch(base, base.Output, "new", true)
	require.NoError(t, err)
	result, err := ApplyShellOutputPatch(nil, checkpoint)
	require.NoError(t, err)
	require.Equal(t, "new", result.Epoch)
	require.EqualValues(t, 1, result.Revision)
	periodic, err := EncodeShellOutputPatch(result, result.Output, "new", true)
	require.NoError(t, err)
	restored, err := ApplyShellOutputPatch(nil, periodic)
	require.NoError(t, err)
	require.Equal(t, result.Output, restored.Output)
	_, err = EncodeShellOutputPatch(nil, ShellExecOutput{Stdout: string([]byte{0xff})}, "epoch", true)
	require.Error(t, err)
	var malformed ShellOutputPatch
	require.Error(t, json.Unmarshal([]byte(`{"stdout":{"operation":"replace","data":"invalid base64"}}`), &malformed))
}

func TestShellOutputPatchOmittedExitPreservesBaseline(t *testing.T) {
	code := 7
	base := &ShellOutputState{Epoch: "epoch", Revision: 1, Output: ShellExecOutput{Stdout: "one", ExitCode: &code}}
	patch, err := EncodeShellOutputPatch(base, ShellExecOutput{Stdout: "one two", ExitCode: &code}, "epoch", false)
	require.NoError(t, err)
	raw, err := json.Marshal(patch)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	delete(fields, "exit_code")
	raw, err = json.Marshal(fields)
	require.NoError(t, err)
	var decoded ShellOutputPatch
	require.NoError(t, json.Unmarshal(raw, &decoded))
	state, err := ApplyShellOutputPatch(base, &decoded)
	require.NoError(t, err)
	require.Equal(t, &code, state.Output.ExitCode)
}

func TestShellOutputPatchExitPresenceRoundTrip(t *testing.T) {
	var omitted ShellOutputPatch
	require.NoError(t, json.Unmarshal([]byte(`{"encoding_version":1}`), &omitted))
	raw, err := json.Marshal(omitted)
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"exit_code"`)
	var unknown ShellOutputPatch
	require.NoError(t, json.Unmarshal([]byte(`{"encoding_version":1,"exit_code":null}`), &unknown))
	raw, err = json.Marshal(unknown)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"exit_code":null`)
}

func TestShellOutputPatchRejectsEpochRewind(t *testing.T) {
	checkpoint, err := EncodeShellOutputPatch(nil, ShellExecOutput{Stdout: "old"}, "epoch", true)
	require.NoError(t, err)
	base := &ShellOutputState{Epoch: "epoch", Revision: 8, Output: ShellExecOutput{Stdout: "latest"}}
	_, err = ApplyShellOutputPatch(base, checkpoint)
	require.Error(t, err, "a fresh checkpoint must use a fresh epoch")
}

func TestShellOutputPatchSaturatedDiverseText(t *testing.T) {
	random := rand.New(rand.NewPCG(17, 29)) //nolint:gosec // Deterministic fixture data.
	var highEntropy strings.Builder
	for range MaxShellOutputFieldBytes {
		highEntropy.WriteByte(byte(32 + random.IntN(95)))
	}
	for name, seed := range map[string]string{
		"incompressible": highEntropy.String(),
		"escaped":        strings.Repeat("\x00\n\t\"", MaxShellOutputFieldBytes/4),
		"multibyte":      strings.Repeat("雪🌍", MaxShellOutputFieldBytes/7),
	} {
		t.Run(name, func(t *testing.T) {
			output := ShellExecOutput{Stdout: seed, Stderr: seed}
			var base *ShellOutputState
			for index := range 40 {
				if index > 0 {
					output.Stdout = output.Stdout[7:] + "雪🌍"
					output.Stderr = output.Stderr[7:] + "\x00\n\ttext"
				}
				// The seed prefix may end inside a multibyte rune only when invalid;
				// all shifts above use seven-byte aligned source seeds.
				patch, err := EncodeShellOutputPatch(base, output, "diverse", false)
				require.NoError(t, err)
				if patch == nil {
					continue
				}
				encoded, err := json.Marshal(patch)
				require.NoError(t, err)
				if index > 0 {
					require.Less(t, len(encoded), 2048)
				}
				base, err = ApplyShellOutputPatch(base, patch)
				require.NoError(t, err)
				require.Equal(t, output, base.Output)
			}
		})
	}
}
