package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-PROMPT-ATTACHMENTS-001.1
func TestMaterializeAttachment_AfterLiveRescan(t *testing.T) {
	root := t.TempDir()
	primary := filepath.Join(root, "primary")
	initWorkspaceGitRepo(t, primary)
	initWorkspaceGitRepo(t, filepath.Join(root, "sibling"))
	log := newTestLogger()
	capturePath := filepath.Join(root, "captured.json")
	cfg := &config.InstanceConfig{WorkDir: primary, Protocol: "acp", AgentArgs: []string{os.Args[0], "-test.run=^TestAttachmentRescanPeerProcess$"}, AgentEnv: []string{"KANDEV_TEST_ATTACHMENT_CAPTURE=" + capturePath}}
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() { require.NoError(t, manager.Stop(context.Background())) })
	require.NoError(t, manager.Start(context.Background()))
	server := NewServer(cfg, manager, nil, nil, log)
	reader := shared.NewAttachmentManager(primary, log.Zap())
	reader.SetSessionID("native-session")
	consumer := manager.GetAdapter()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, consumer.Initialize(ctx))
	_, err := consumer.NewSession(ctx, nil)
	require.NoError(t, err)
	var content bytes.Buffer
	require.NoError(t, png.Encode(&content, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	names := make(map[string]bool)
	for i := range 3 {
		if i > 0 {
			require.NoError(t, manager.RescanRepositories(context.Background(), root))
		}
		result := uploadRescanAttachment(t, server, content.Bytes())
		require.Equal(t, filepath.Join(primary, ".kandev", "attachments", "native-session"), filepath.Dir(result.Path), "materializer must use the active agent directory")
		require.False(t, names[result.Name], "uploads must not overwrite an earlier attachment")
		names[result.Name] = true
		saved, err := reader.SaveAttachments([]v1.MessageAttachment{{Type: "image", AttachmentID: "image-id", Name: result.Name, MimeType: "image/png", SizeBytes: int64(content.Len())}})
		require.NoError(t, err, "upload %d must remain readable from the active agent directory", i)
		got, err := os.ReadFile(saved[0].AbsPath)
		require.NoError(t, err)
		require.Equal(t, content.Bytes(), got)
		require.NoError(t, consumer.Prompt(ctx, "", []v1.MessageAttachment{{Type: "image", AttachmentID: "image-id", Name: result.Name, MimeType: "image/png"}}, uint64(i+1)))
		captured, err := os.ReadFile(capturePath)
		require.NoError(t, err)
		var request sdk.PromptRequest
		require.NoError(t, json.Unmarshal(captured, &request))
		require.Equal(t, base64.StdEncoding.EncodeToString(content.Bytes()), request.Prompt[1].Image.Data)
	}
}

type attachmentRescanPeer struct {
	sdk.Agent
	capturePath string
}

func (f *attachmentRescanPeer) Initialize(_ context.Context, req sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	return sdk.InitializeResponse{ProtocolVersion: req.ProtocolVersion}, nil
}

func (f *attachmentRescanPeer) NewSession(context.Context, sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	return sdk.NewSessionResponse{SessionId: "native-session"}, nil
}

func (f *attachmentRescanPeer) Prompt(_ context.Context, req sdk.PromptRequest) (sdk.PromptResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return sdk.PromptResponse{}, err
	}
	err = os.WriteFile(f.capturePath, data, 0o600)
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, err
}

// TestAttachmentRescanPeerProcess serves ACP in the manager-owned subprocess.
func TestAttachmentRescanPeerProcess(t *testing.T) {
	path := os.Getenv("KANDEV_TEST_ATTACHMENT_CAPTURE")
	if path == "" {
		return
	}
	peer := &attachmentRescanPeer{capturePath: path}
	<-sdk.NewAgentSideConnection(peer, os.Stdout, os.Stdin).Done()
	os.Exit(0)
}

func uploadRescanAttachment(t *testing.T, server *Server, data []byte) materializedAttachmentResponse {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"session_id": "native-session", "attachment_id": "image-id", "name": "image.png", "mime_type": "image/png", "size_bytes": strconv.Itoa(len(data))} {
		require.NoError(t, writer.WriteField(key, value))
	}
	part, err := writer.CreateFormFile("file", "image.png")
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/attachments/materialize", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, req)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var result materializedAttachmentResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	return result
}

func TestMaterializeAttachment_TeardownReportsAgentUnavailable(t *testing.T) {
	cfg := &config.InstanceConfig{WorkDir: t.TempDir()}
	log := newTestLogger()
	manager := process.NewManager(cfg, log)
	server := NewServer(cfg, manager, nil, nil, log)
	manager.CloseAdmission()
	_, err := server.persistMaterializedAttachment(materializedAttachmentUpload{})
	status, message, ok := materializedAttachmentError(err)
	require.True(t, ok)
	require.Equal(t, http.StatusServiceUnavailable, status)
	require.Equal(t, "attachment agent unavailable", message)
}
