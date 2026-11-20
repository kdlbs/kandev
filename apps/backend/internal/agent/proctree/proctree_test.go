package proctree

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

const helperModeEnv = "KANDEV_PROCTREE_TEST_HELPER"

// TestMain doubles as a helper process: "parent" starts a long-lived
// descendant that inherits stdout, prints its PID, and then either sleeps or
// exits, leaving the descendant behind.
func TestMain(m *testing.M) {
	switch os.Getenv(helperModeEnv) {
	case "":
		os.Exit(m.Run())
	case "descendant":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "parent", "orphaning-parent":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), helperModeEnv+"=descendant")
		child.Stdout = os.Stdout
		if err := child.Start(); err != nil {
			os.Exit(3)
		}
		_, _ = os.Stdout.WriteString(strconv.Itoa(child.Process.Pid) + "\n")
		if os.Getenv(helperModeEnv) == "parent" {
			time.Sleep(time.Minute)
		}
		os.Exit(0)
	}
	os.Exit(4)
}

type helperTree struct {
	cmd    *exec.Cmd
	tree   Tree
	output *os.File
}

func startHelperTree(t *testing.T, mode string) helperTree {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), helperModeEnv+"="+mode)
	cmd.Stdout = writer
	Prepare(cmd)
	if err := cmd.Start(); err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	_ = writer.Close()
	tree, err := Capture(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("capture tree: %v", err)
	}
	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil {
		tree.Kill(cmd)
		_ = cmd.Wait()
		t.Fatalf("read descendant pid: %v", err)
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(line)); err != nil || pid <= 0 {
		t.Fatalf("descendant pid = %q", line)
	}
	return helperTree{cmd: cmd, tree: tree, output: reader}
}

// requireOutputClosed proves that no process in the tree still holds the
// inherited stdout pipe. It works the same with process groups and jobs.
func requireOutputClosed(t *testing.T, output *os.File) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, output)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("drain output: %v", err)
		}
	case <-time.After(10 * time.Second):
		_ = output.Close()
		t.Fatal("a descendant survived tree kill and still holds the output pipe")
	}
}

func TestKillTerminatesRunningParentAndDescendant(t *testing.T) {
	helper := startHelperTree(t, "parent")
	helper.tree.Kill(helper.cmd)
	requireOutputClosed(t, helper.output)
	_ = helper.cmd.Wait()
}

func TestKillAfterParentExitTerminatesDescendant(t *testing.T) {
	helper := startHelperTree(t, "orphaning-parent")
	if err := helper.cmd.Wait(); err != nil {
		t.Fatalf("parent exit: %v", err)
	}
	helper.tree.Kill(helper.cmd)
	requireOutputClosed(t, helper.output)
}

func TestKillToleratesUnstartedCommand(t *testing.T) {
	var tree Tree
	tree.Kill(nil)
	tree.Kill(exec.Command(os.Args[0]))
}
