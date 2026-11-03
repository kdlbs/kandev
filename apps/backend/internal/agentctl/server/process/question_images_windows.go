package process

import "os"

func openQuestionImageFile(root *os.Root, path string) (*os.File, error) {
	return root.Open(path)
}
