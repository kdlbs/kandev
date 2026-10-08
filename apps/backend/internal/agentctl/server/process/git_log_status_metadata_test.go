package process

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type statusMetadataCase struct {
	name, path, patch, status string
	additions, deletions      int
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.1
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.2
func TestParseCommitDiffWithOptions_StatusMetadata(t *testing.T) {
	op := NewGitOperator("/workspace", newTestLogger(t), nil)
	for _, tc := range append(statusMetadataNegativeCases(), statusMetadataPositiveCases()...) {
		t.Run(tc.name, func(t *testing.T) {
			files := op.parseCommitDiffWithOptions(tc.patch, parseCommitDiffOptions{})
			want := map[string]interface{}{
				tc.path: map[string]interface{}{
					"path": tc.path, "status": tc.status, "staged": false,
					"additions": tc.additions, "deletions": tc.deletions, "diff": tc.patch,
				},
			}
			if !reflect.DeepEqual(files, want) {
				t.Errorf("parsed files = %#v, want %#v", files, want)
			}
		})
	}
}

func statusMetadataNegativeCases() []statusMetadataCase {
	var cases []statusMetadataCase
	for _, marker := range []string{"new file mode 100644", "deleted file mode 100644", "rename from unrelated.txt"} {
		for _, quoted := range []bool{false, true} {
			path := marker + ".txt"
			if quoted {
				path += "\tquoted"
			}
			cases = append(cases, statusMetadataCase{
				name: fmt.Sprintf("path %s quoted=%t", marker, quoted), path: path,
				patch:  statusMetadataPatch(path, "index 1111111..2222222 100644\n", "@@ -1 +1 @@\n-old\n+new\n"),
				status: "modified", additions: 1, deletions: 1,
			})
		}
		for _, prefix := range []string{"+", "-", " ", " \t", "  "} {
			body := "@@ -1,2 +1,2 @@\n" + prefix + marker + "\n-old\n+new\n"
			additions, deletions := 1, 1
			if prefix == "+" {
				additions++
			}
			if prefix == "-" {
				deletions++
			}
			cases = append(cases, statusMetadataCase{
				name: "body " + prefix + marker, path: "ordinary.txt",
				patch:  statusMetadataPatch("ordinary.txt", "", body),
				status: "modified", additions: additions, deletions: deletions,
			})
		}
		cases = append(cases, statusMetadataCase{
			name: "hunk description " + marker, path: "ordinary.txt",
			patch:  statusMetadataPatch("ordinary.txt", "", "@@ -1 +1 @@ "+marker+"\n-old\n+new\n"),
			status: "modified", additions: 1, deletions: 1,
		})
	}
	for _, metadata := range []string{"new file mode", "new file mode 10064", "new file mode 100648", "new file mode 100644 extra", "deleted file mode 100644x", "rename from", "rename from ", " new file mode 100644"} {
		cases = append(cases, statusMetadataCase{
			name: "invalid " + metadata, path: "ordinary.txt",
			patch:  statusMetadataPatch("ordinary.txt", metadata+"\n", "@@ -1 +1 @@\n-old\n+new\n"),
			status: "modified", additions: 1, deletions: 1,
		})
	}
	for _, boundary := range []string{"--- a/ordinary.txt", "+++ b/ordinary.txt", "@@ -1 +1 @@", "Binary files a/ordinary.txt and b/ordinary.txt differ", "GIT binary patch"} {
		cases = append(cases, statusMetadataCase{
			name: "payload boundary " + boundary, path: "ordinary.txt",
			patch:  "diff --git a/ordinary.txt b/ordinary.txt\n" + boundary + "\nnew file mode 100644\n",
			status: "modified",
		})
	}
	return cases
}

func statusMetadataPositiveCases() []statusMetadataCase {
	cases := []statusMetadataCase{
		{name: "ordinary", path: "ordinary.txt", patch: statusMetadataPatch("ordinary.txt", "", "@@ -1 +1 @@\n-old\n+new\n"), status: "modified", additions: 1, deletions: 1},
		{name: "mode only marker path", path: "new file mode.txt", patch: "diff --git a/new file mode.txt b/new file mode.txt\nold mode 100644\nnew mode 100755\n", status: "modified"},
		{name: "binary marker path", path: "deleted file mode.bin", patch: "diff --git a/deleted file mode.bin b/deleted file mode.bin\nindex 1111111..2222222 100644\nBinary files a/deleted file mode.bin and b/deleted file mode.bin differ\n", status: "modified"},
		{name: "pure rename competing markers", path: "deleted file mode.txt", patch: "diff --git a/new file mode.txt b/deleted file mode.txt\nsimilarity index 100%\nrename from new file mode.txt\nrename to deleted file mode.txt\n", status: "renamed"},
		{name: "edited rename", path: "new file mode.txt", patch: "diff --git a/old.txt b/new file mode.txt\nsimilarity index 90%\nrename from old.txt\nrename to new file mode.txt\n--- a/old.txt\n+++ b/new file mode.txt\n@@ -1 +1 @@\n-old\n+deleted file mode 100644\n", status: "renamed", additions: 1, deletions: 1},
	}
	for _, mode := range []string{"100644", "100755", "120000", "160000"} {
		for _, status := range []string{"added", "deleted"} {
			metadata := "new file mode " + mode
			if status == "deleted" {
				metadata = "deleted file mode " + mode
			}
			cases = append(cases, statusMetadataCase{
				name: status + " empty " + mode, path: "rename from.txt",
				patch:  "diff --git a/rename from.txt b/rename from.txt\n" + metadata,
				status: status,
			})
			cases = append(cases, statusMetadataCase{
				name: status + " binary " + mode, path: "rename from.bin",
				patch:  "diff --git a/rename from.bin b/rename from.bin\n" + metadata + "\nGIT binary patch\nliteral 0\nHcmV?d00001\n",
				status: status,
			})
		}
	}
	return cases
}

func statusMetadataPatch(path, metadata, body string) string {
	a, b := "a/"+path, "b/"+path
	if strings.ContainsAny(path, "\t\n\"\\") {
		a, b = strconv.Quote(a), strconv.Quote(b)
	}
	return "diff --git " + a + " " + b + "\n" + metadata + "--- " + a + "\n+++ " + b + "\n" + body
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.4
func TestParseCommitDiffWithOptions_StatusMetadataBudgets(t *testing.T) {
	first := statusMetadataPatch("new file mode.txt", "", "@@ -1 +1 @@\n-old\n+rename from unrelated.txt\n")
	second := "diff --git a/new file mode old.txt b/moved.txt\nsimilarity index 100%\nrename from new file mode old.txt\nrename to moved.txt\n"
	op := NewGitOperator("/workspace", newTestLogger(t), nil)
	files := op.parseCommitDiffWithOptions(first+second, parseCommitDiffOptions{perFileMaxBytes: 20, totalMaxBytes: 20})
	want := map[string]interface{}{
		"new file mode.txt": map[string]interface{}{"path": "new file mode.txt", "status": "modified", "staged": false, "additions": 1, "deletions": 1, "diff": first[:20], "diff_skip_reason": diffSkipReasonTruncated},
		"moved.txt":         map[string]interface{}{"path": "moved.txt", "status": "renamed", "staged": false, "additions": 0, "deletions": 0, "diff": "", "diff_skip_reason": diffSkipReasonBudgetExceeded},
	}
	if !reflect.DeepEqual(files, want) {
		t.Errorf("budgeted files = %#v, want %#v", files, want)
	}
}
