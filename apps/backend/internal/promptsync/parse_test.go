package promptsync

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	promptservice "github.com/kandev/kandev/internal/prompts/service"
	"github.com/kandev/kandev/internal/reposync"
)

func TestSelectPromptFilesKeepsImmediateMarkdownFiles(t *testing.T) {
	entries := []reposync.Entry{
		{Name: "b.md", Path: "p/b.md", IsFile: true},
		{Name: "A.MD", Path: "p/A.MD", IsFile: true},
		{Name: "notes.txt", Path: "p/notes.txt", IsFile: true},
		{Name: "nested.md", Path: "p/nested.md", IsDir: true},
		{Name: "md", Path: "p/md", IsFile: true},
	}
	assert.Equal(t, []string{"p/A.MD", "p/b.md"}, selectPromptFiles(entries))
}

func TestParsePromptFiles(t *testing.T) {
	tooLong := strings.Repeat("x", promptservice.MaxPromptContentBytes+1)
	longName := strings.Repeat("n", promptservice.MaxPromptNameBytes+1)
	desired, frozen, warnings := parsePromptFiles([]fetchedFile{
		{path: "p/principles.md", content: []byte("\n  Be simple.  \n")},
		{path: "p/empty.md", content: []byte("   \n")},
		{path: "p/binary.md", content: []byte{0xff, 0xfe}},
		{path: "p/huge.md", content: []byte(tooLong)},
		{path: "p/" + longName + ".md", content: []byte("ok")},
		{path: "p/ spaced.md", content: []byte("ok")},
		{path: "p/.md", content: []byte("ok")},
		{path: "p/dup.md", content: []byte("one")},
		{path: "p/dup.MD", content: []byte("two")},
	})
	assert.Equal(t, []promptservice.SyncedPromptFile{
		{Name: "principles", Content: "Be simple.", Path: "p/principles.md"},
	}, desired)
	assert.True(t, frozen["empty"])
	assert.True(t, frozen["binary"])
	assert.True(t, frozen["huge"])
	assert.True(t, frozen[" spaced"])
	assert.True(t, frozen["dup"])
	assert.False(t, frozen[longName], "an invalid name cannot own a prompt")
	assert.Len(t, warnings, 8)
}
