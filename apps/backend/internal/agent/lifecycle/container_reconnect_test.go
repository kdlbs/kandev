package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/docker"
)

func newTestContainerMgr() *ContainerManager {
	return &ContainerManager{logger: newTestDockerLogger()}
}

func TestValidateContainerOwnership_NotManaged(t *testing.T) {
	cm := newTestContainerMgr()
	info := &docker.ContainerInfo{Labels: map[string]string{}}

	err := cm.validateContainerOwnership(info, "abc", "task-1")
	if err == nil {
		t.Fatal("expected error for unmanaged container")
	}
}

func TestValidateContainerOwnership_ManagedWrongTask(t *testing.T) {
	cm := newTestContainerMgr()
	info := &docker.ContainerInfo{Labels: map[string]string{
		"kandev.managed": "true",
		"kandev.task_id": "task-other",
	}}

	err := cm.validateContainerOwnership(info, "abc", "task-1")
	if err == nil {
		t.Fatal("expected error for container belonging to a different task")
	}
}

func TestValidateContainerOwnership_ManagedMissingTaskID(t *testing.T) {
	cm := newTestContainerMgr()
	info := &docker.ContainerInfo{Labels: map[string]string{
		"kandev.managed": "true",
	}}

	err := cm.validateContainerOwnership(info, "abc", "task-1")
	if err == nil {
		t.Fatal("expected error for managed container with missing kandev.task_id label")
	}
}

func TestValidateContainerOwnership_ManagedCorrectTask(t *testing.T) {
	cm := newTestContainerMgr()
	info := &docker.ContainerInfo{Labels: map[string]string{
		"kandev.managed": "true",
		"kandev.task_id": "task-1",
	}}

	if err := cm.validateContainerOwnership(info, "abc", "task-1"); err != nil {
		t.Fatalf("unexpected error for valid container: %v", err)
	}
}

func TestValidateContainerOwnership_NilLabels(t *testing.T) {
	cm := newTestContainerMgr()
	info := &docker.ContainerInfo{Labels: nil}

	err := cm.validateContainerOwnership(info, "abc", "task-1")
	if err == nil {
		t.Fatal("expected error when labels are nil")
	}
}
