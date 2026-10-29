//go:build !unix

package journal

func classifyOpenFilesystemCause(error) OpenReason { return "" }
