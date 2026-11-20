package acp

import "os"

func openClaudeMonitorContainedFile(root *os.Root, name string) (*os.File, error) {
	return root.Open(name)
}
