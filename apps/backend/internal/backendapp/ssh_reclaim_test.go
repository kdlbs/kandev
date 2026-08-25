package backendapp

import (
	"context"
	"strings"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type fakeRuntimeSSHTaskDirReclaimer struct {
	request agentruntime.SSHTaskDirReclaimRequest
	result  agentruntime.SSHTaskDirReclaimResult
	err     error
}

func (f *fakeRuntimeSSHTaskDirReclaimer) ReclaimTaskDir(
	_ context.Context,
	req agentruntime.SSHTaskDirReclaimRequest,
) (agentruntime.SSHTaskDirReclaimResult, error) {
	f.request = req
	return f.result, f.err
}

func TestSSHTaskDirReclaimerAdapterRejectsMissingFingerprint(t *testing.T) {
	adapter := newSSHTaskDirReclaimerAdapter(nil)
	_, err := adapter.ReclaimTaskDir(context.Background(), taskservice.SSHTaskDirReclaimRequest{
		Host: "build.example",
	})
	if err == nil || !strings.Contains(err.Error(), "no pinned host fingerprint") {
		t.Fatalf("ReclaimTaskDir error = %v, want missing fingerprint error", err)
	}
}

func TestSSHTaskDirReclaimerAdapterTranslatesRequestAndResult(t *testing.T) {
	fake := &fakeRuntimeSSHTaskDirReclaimer{
		result: agentruntime.SSHTaskDirReclaimResult{
			Removed:    false,
			SkipReason: "unpushed_commits",
			Detail:     "2 commits",
		},
	}
	adapter := &sshTaskDirReclaimerAdapter{reclaimer: fake}
	req := taskservice.SSHTaskDirReclaimRequest{
		Host:            "build.example",
		Port:            22,
		User:            "builder",
		HostFingerprint: "SHA256:pinned",
		IdentitySource:  "file",
		IdentityFile:    "/home/builder/.ssh/id_ed25519",
		ProxyJump:       "bastion.example",
		Shell:           "bash",
		WorkdirRoot:     "/srv/work",
		TaskDir:         "/srv/work/task",
	}

	result, err := adapter.ReclaimTaskDir(context.Background(), req)
	if err != nil {
		t.Fatalf("ReclaimTaskDir: %v", err)
	}
	if fake.request.Host != req.Host || fake.request.Port != req.Port || fake.request.User != req.User ||
		fake.request.HostFingerprint != req.HostFingerprint || fake.request.IdentitySource != req.IdentitySource ||
		fake.request.IdentityFile != req.IdentityFile || fake.request.ProxyJump != req.ProxyJump ||
		fake.request.Shell != req.Shell || fake.request.WorkdirRoot != req.WorkdirRoot || fake.request.TaskDir != req.TaskDir {
		t.Fatalf("runtime request = %+v, want %+v", fake.request, req)
	}
	if result != (taskservice.SSHTaskDirReclaimResult{SkipReason: "unpushed_commits", Detail: "2 commits"}) {
		t.Fatalf("task service result = %+v, want safety skip", result)
	}
}
