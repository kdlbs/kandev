package client

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestRequestFileTreeReadFailureAndEmptyDirectory(t *testing.T) {
	for _, failed := range []bool{true, false} {
		name := "empty directory"
		status := http.StatusOK
		body := types.FileTreeResponse{Root: &types.FileTreeNode{Path: "src", Name: "src", IsDir: true}}
		if failed {
			name = "directory read failure"
			status = http.StatusBadRequest
			body = types.FileTreeResponse{Error: "read directory: filesystem unavailable"}
		}
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			server, _ := captureServer(t, jsonResponder(status, string(encoded)))
			reply, err := newHTTPOnlyClient(server.URL).RequestFileTree(context.Background(), "src", 1)
			if failed {
				if err == nil || reply != nil {
					t.Fatalf("failed read = (%+v, %v), want rejected reply", reply, err)
				}
				return
			}
			if err != nil || reply == nil || reply.Root == nil || !reply.Root.IsDir || len(reply.Root.Children) != 0 {
				t.Fatalf("empty read = (%+v, %v), want successful empty directory", reply, err)
			}
		})
	}
}
