package providers

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func fakeApprise(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(source, []byte(`package main
import("os";"encoding/json";"fmt")
func main(){data,_:=json.Marshal(os.Args[1:]);_ = os.WriteFile(os.Getenv("APPRISE_TEST_CAPTURE"),data,0600);if os.Getenv("APPRISE_TEST_FAIL")=="1"{fmt.Fprintln(os.Stderr,"synthetic-receiver-secret");os.Exit(1)}}
`), 0600))
	binary := filepath.Join(dir, "apprise")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	command := exec.Command("go", "build", "-trimpath", "-o", binary, source)
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	capture := filepath.Join(dir, "arguments.json")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("APPRISE_TEST_CAPTURE", capture)
	return capture
}

func TestAppriseContentIsLiteralAndErrorsDoNotExposeOutput(t *testing.T) {
	capture := fakeApprise(t)
	provider := NewAppriseProvider()
	message := Message{Title: "--help", Body: "--attach=private-file", Config: map[string]interface{}{"urls": []string{"json://example.invalid"}}}
	require.NoError(t, provider.Send(context.Background(), message))
	raw, err := os.ReadFile(capture)
	require.NoError(t, err)
	var arguments []string
	require.NoError(t, json.Unmarshal(raw, &arguments))
	require.Equal(t, []string{"--title=--help", "--body=--attach=private-file", "--", "json://example.invalid"}, arguments)
	t.Setenv("APPRISE_TEST_FAIL", "1")
	err = provider.Send(context.Background(), message)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "synthetic-receiver-secret")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, provider.Send(ctx, message), context.Canceled)
}
