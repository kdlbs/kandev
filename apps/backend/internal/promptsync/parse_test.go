package promptsync

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		{Name: ".md", Path: "p/.md", IsFile: true},
	}
	assert.Equal(t, []string{"p/.md", "p/A.MD", "p/b.md"}, selectPromptFiles(entries),
		"an empty-name .md file is selected so validation can warn about it")
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

func TestParsePromptFilesInvalidDuplicateFreezesValidTwin(t *testing.T) {
	desired, frozen, warnings := parsePromptFiles([]fetchedFile{
		{path: "p/review.md", content: []byte("valid")},
		{path: "p/review.MD", content: []byte("   ")},
	})
	assert.Empty(t, desired, "a frozen name must not be applied")
	assert.True(t, frozen["review"])
	joined := strings.Join(warnings, "\n")
	assert.Contains(t, joined, "another file also defines prompt")
}

func TestParsePromptFilesTooLargeFileIsFrozenWithWarning(t *testing.T) {
	desired, frozen, warnings := parsePromptFiles([]fetchedFile{
		{path: "p/big.md", tooLarge: true},
		{path: "p/ok.md", content: []byte("fine")},
	})
	assert.Len(t, desired, 1)
	assert.True(t, frozen["big"])
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "p/big.md")
}

func TestParsePromptFilesInvalidDuplicateFreezesInEitherOrder(t *testing.T) {
	desired, frozen, _ := parsePromptFiles([]fetchedFile{
		{path: "p/review.MD", content: []byte("   ")},
		{path: "p/review.md", content: []byte("valid")},
	})
	assert.Empty(t, desired)
	assert.True(t, frozen["review"])

	desired, frozen, _ = parsePromptFiles([]fetchedFile{{path: "p/review.md", content: []byte("valid")}})
	assert.Len(t, desired, 1, "a single valid file still applies")
	assert.False(t, frozen["review"])
}

func TestParsePromptFilesFreezesAFileWithANulCharacter(t *testing.T) {
	desired, frozen, warnings := parsePromptFiles([]fetchedFile{
		{path: "p/broken.md", content: []byte("before\x00after")},
		{path: "p/valid.md", content: []byte("Valid")},
	})
	assert.Equal(t, []promptservice.SyncedPromptFile{{Name: "valid", Content: "Valid", Path: "p/valid.md"}}, desired)
	assert.True(t, frozen["broken"])
	assert.Len(t, warnings, 1)
}

func TestCompleteListingRejectsACappedGitHubDirectory(t *testing.T) {
	full := make([]reposync.Entry, githubContentsListLimit)
	assert.ErrorIs(t, completeListing(reposync.ProviderGitHub, full), ErrListingIncomplete)
	assert.NoError(t, completeListing(reposync.ProviderGitHub, full[:githubContentsListLimit-1]))
	assert.NoError(t, completeListing(reposync.ProviderGitLab, full), "GitLab listings are paginated to the end")
	assert.Equal(t, ErrListingIncomplete.Error(), safeErrorMessage(ErrListingIncomplete))
	assert.Equal(t, ErrListingIncomplete.Error(), safeStoredMessage(ErrListingIncomplete.Error()))
}
