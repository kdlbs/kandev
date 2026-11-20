package websocket

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-002.8
func TestRuntimeShim_PreservesWebGLDrawingBuffers(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "")
	require.Equal(t, 1, strings.Count(shim, webglPreserveShimJS), "the WebGL wrapper is installed exactly once")
	require.Less(t, strings.Index(shim, webglPreserveShimJS), strings.LastIndex(shim, "})();"),
		"the wrapper runs inside the shim IIFE")
}

// TestWebGLPreserveShimBehavior executes the wrapper against a fake canvas
// prototype. It needs node, which local development and the web CI provide;
// the Go-only CI job skips it and relies on the Playwright capture test.
func TestWebGLPreserveShimBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	harness := `
var calls = [];
function FakeCanvas() {}
FakeCanvas.prototype.getContext = function (type, attrs) { calls.push([type, attrs]); return { type: type }; };
var window = { HTMLCanvasElement: FakeCanvas };
var callerAttrs = { antialias: false, preserveDrawingBuffer: false };
` + webglPreserveShimJS + webglPreserveShimJS + `
var c = new FakeCanvas();
var gl = c.getContext('webgl2', callerAttrs);
c.getContext('webgl');
c.getContext('2d', { alpha: false });
var out = {
  gl: gl.type,
  webgl2: calls[0][1],
  webgl: calls[1][1],
  twoD: calls[2][1],
  callerUnchanged: callerAttrs.preserveDrawingBuffer === false,
  wrappedOnce: calls.length === 3,
};
process.stdout.write(JSON.stringify(out));
`
	path := filepath.Join(t.TempDir(), "harness.js")
	require.NoError(t, os.WriteFile(path, []byte(harness), 0o600))
	output, err := exec.Command(node, path).CombinedOutput()
	require.NoError(t, err, string(output))
	require.JSONEq(t, `{
		"gl": "webgl2",
		"webgl2": {"antialias": false, "preserveDrawingBuffer": true},
		"webgl": {"preserveDrawingBuffer": true},
		"twoD": {"alpha": false},
		"callerUnchanged": true,
		"wrappedOnce": true
	}`, string(output))
}
