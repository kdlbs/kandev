package process

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGenericWrapperInteractiveInputAndResize(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	for _, immediate := range []bool{true, false} {
		t.Run(map[bool]string{true: "immediate", false: "resize"}[immediate], func(t *testing.T) {
			dir := t.TempDir()
			wrapper, child := filepath.Join(dir, "wrapper with spaces"), filepath.Join(dir, "child")
			receipt, input := filepath.Join(dir, "receipt"), filepath.Join(dir, "input")
			require.NoError(t, os.WriteFile(wrapper, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$RECEIPT\"\nshift\nexec \"$@\"\n"), 0700))
			require.NoError(t, os.WriteFile(child, []byte("#!/bin/sh\nIFS= read -r line\nprintf '%s' \"$line\" > \"$INPUT\"\n"), 0700))
			data, err := json.Marshal(map[string]any{"session_id": "wrapped-session", "command": []string{child, "", "argument with spaces"}, "working_dir": dir,
				"env": map[string]string{"RECEIPT": receipt, "INPUT": input}, "immediate_start": immediate,
				"launch_wrapper": map[string]any{"version": 1, "source": "profile", "argv": []string{wrapper, "{{kandev.work_dir}}"}},
			})
			require.NoError(t, err)
			var req InteractiveStartRequest
			require.NoError(t, json.Unmarshal(data, &req))
			runner := NewInteractiveRunner(nil, newTestLogger(t), 1024)
			info, err := runner.Start(context.Background(), req)
			require.NoError(t, err)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = runner.Stop(ctx, info.ID)
			})
			require.NoError(t, runner.ResizeByProcessID(info.ID, 90, 30))
			writer, err := runner.GetPtyWriter(info.ID)
			require.NoError(t, err)
			_, err = writer.Write([]byte("hello wrapper\n"))
			require.NoError(t, err)
			require.Eventually(t, func() bool { got, err := os.ReadFile(input); return err == nil && string(got) == "hello wrapper" }, 3*time.Second, 10*time.Millisecond)
			got, err := os.ReadFile(receipt)
			require.NoError(t, err, "native input success must also prove wrapper execution")
			require.Equal(t, dir+"\n"+child+"\n\nargument with spaces\n", string(got))
		})
	}
}
