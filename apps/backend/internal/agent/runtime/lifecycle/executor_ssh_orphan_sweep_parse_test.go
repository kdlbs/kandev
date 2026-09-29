package lifecycle

import (
	"testing"
)

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.13
func TestRemoteCommandLineFlagValue(t *testing.T) {
	cases := []struct {
		name      string
		line      string
		flag      string
		wantValue string
		wantOK    bool
	}{
		{
			name:      "space separated value",
			line:      "/opt/kandev/bin/agentctl --workdir /home/user/.kandev/tasks/task-abc",
			flag:      "--workdir",
			wantValue: "/home/user/.kandev/tasks/task-abc",
			wantOK:    true,
		},
		{
			name:      "equals separated value",
			line:      "/opt/kandev/bin/agentctl --workdir=/home/user/.kandev/tasks/task-abc --port 9",
			flag:      "--workdir",
			wantValue: "/home/user/.kandev/tasks/task-abc",
			wantOK:    true,
		},
		{
			name:   "flag absent",
			line:   "/opt/kandev/bin/agentctl --port 9",
			flag:   "--workdir",
			wantOK: false,
		},
		{
			name:   "flag with empty value",
			line:   "/opt/kandev/bin/agentctl --workdir ",
			flag:   "--workdir",
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, ok := remoteCommandLineFlagValue(tc.line, tc.flag)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && value != tc.wantValue {
				t.Fatalf("value = %q, want %q", value, tc.wantValue)
			}
		})
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.13
func TestSSHOrphanTaskIDFromWorkdir(t *testing.T) {
	const root = "/home/user/.kandev/tasks"
	cases := []struct {
		name        string
		workdir     string
		wantTaskDir string
		wantTaskID  string
		wantOK      bool
	}{
		{
			name:        "exact task dir matches",
			workdir:     root + "/task-abc-123",
			wantTaskDir: "task-abc-123",
			wantTaskID:  "abc-123",
			wantOK:      true,
		},
		{
			name:    "subdirectory of a task dir is ignored",
			workdir: root + "/task-abc-123/nested",
			wantOK:  false,
		},
		{
			name:    "outside the configured root is ignored",
			workdir: "/somewhere/else/task-abc-123",
			wantOK:  false,
		},
		{
			name:    "empty task id is ignored",
			workdir: root + "/task-",
			wantOK:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			taskDir, taskID, ok := sshOrphanTaskIDFromWorkdir(root, tc.workdir)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if taskDir != tc.wantTaskDir {
				t.Fatalf("taskDir = %q, want %q", taskDir, tc.wantTaskDir)
			}
			if taskID != tc.wantTaskID {
				t.Fatalf("taskID = %q, want %q", taskID, tc.wantTaskID)
			}
		})
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.13, AC-EXECUTORS-SSH-EXECUTOR-001.15
func TestParseSSHOrphanInventory(t *testing.T) {
	const root = "/home/user/.kandev"
	output := "" +
		"PROC\t111\t1\t/home/user/.kandev/bin/agentctl --workdir /home/user/.kandev/tasks/task-alive\n" +
		"PROC\t222\t1\t/home/user/.kandev/bin/agentctl --workdir /home/user/.kandev/tasks/task-orphan\n" +
		"PROC\t333\t1\tsomething-else --workdir /home/user/.kandev/tasks/task-other-proc\n" +
		"PROC\t444\t1\t/home/user/.kandev/bin/agentctl --workdir /elsewhere/tasks/task-outside-root\n" +
		"PIDFILE\ttask-alive\tsession-1\t111\n" +
		"PIDFILE\ttask-tainted\tsession-2\tnot-a-number\n" +
		"garbage line that matches neither prefix\n" +
		"PIDFILE\ttask-alive\n"

	inv := parseSSHOrphanInventory(output, root)

	if len(inv.Processes) != 2 {
		t.Fatalf("len(Processes) = %d, want 2 (only agentctl processes under the root): %+v", len(inv.Processes), inv.Processes)
	}
	byPID := map[int]sshOrphanProcessRecord{}
	for _, p := range inv.Processes {
		byPID[p.PID] = p
	}
	alive, ok := byPID[111]
	if !ok {
		t.Fatalf("expected pid 111 to be discovered, got %+v", inv.Processes)
	}
	if alive.TaskDir != "task-alive" || alive.TaskID != "alive" || alive.PPID != 1 {
		t.Fatalf("pid 111 record = %+v, want TaskDir=task-alive TaskID=alive PPID=1", alive)
	}
	orphan, ok := byPID[222]
	if !ok || orphan.TaskDir != "task-orphan" {
		t.Fatalf("expected pid 222 attributed to task-orphan, got %+v ok=%v", orphan, ok)
	}
	if _, ok := byPID[333]; ok {
		t.Fatal("a command line that does not contain \"agentctl\" must not be discovered")
	}
	if _, ok := byPID[444]; ok {
		t.Fatal("a workdir outside the configured root must not be discovered")
	}

	sessionID, claimed := sshOrphanAttributeSession(inv, alive)
	if !claimed || sessionID != "session-1" {
		t.Fatalf("sshOrphanAttributeSession(alive) = (%q, %v), want (session-1, true)", sessionID, claimed)
	}
	_, claimed = sshOrphanAttributeSession(inv, orphan)
	if claimed {
		t.Fatal("task-orphan has no pidfile claim, so orphan must be unclaimed")
	}

	if !inv.taintedTasks["task-tainted"] {
		t.Fatal("a pidfile with unparsable content must taint its task dir")
	}
	if inv.taintedTasks["task-alive"] {
		t.Fatal("a well-formed pidfile line must not taint its task dir")
	}
}
