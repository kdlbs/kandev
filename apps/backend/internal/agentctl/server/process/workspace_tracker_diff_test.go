package process

import (
	"testing"

	"github.com/sourcegraph/go-diff/diff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyDiffToContent_SingleHunk(t *testing.T) {
	original := `line1
line2
line3
line4
line5`

	// Diff that modifies line3
	diffStr := `--- a/test.txt
+++ b/test.txt
@@ -1,5 +1,5 @@
 line1
 line2
-line3
+line3_modified
 line4
 line5`

	expected := `line1
line2
line3_modified
line4
line5`

	fileDiffs, err := diff.ParseMultiFileDiff([]byte(diffStr))
	require.NoError(t, err)
	require.Len(t, fileDiffs, 1)

	result, err := applyDiffToContent(original, fileDiffs[0])
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestApplyDiffToContent_MultipleHunks(t *testing.T) {
	original := `line1
line2
line3
line4
line5
line6
line7
line8
line9
line10`

	// Diff with two separate hunks
	diffStr := `--- a/test.txt
+++ b/test.txt
@@ -1,4 +1,4 @@
 line1
-line2
+line2_modified
 line3
 line4
@@ -7,4 +7,4 @@
 line7
 line8
-line9
+line9_modified
 line10`

	expected := `line1
line2_modified
line3
line4
line5
line6
line7
line8
line9_modified
line10`

	fileDiffs, err := diff.ParseMultiFileDiff([]byte(diffStr))
	require.NoError(t, err)
	require.Len(t, fileDiffs, 1)

	result, err := applyDiffToContent(original, fileDiffs[0])
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestApplyDiffToContent_AdditionsAndDeletions(t *testing.T) {
	original := `line1
line2
line3
line4
line5`

	// Diff that adds and deletes lines
	diffStr := `--- a/test.txt
+++ b/test.txt
@@ -1,5 +1,6 @@
 line1
+new_line
 line2
-line3
 line4
 line5`

	expected := `line1
new_line
line2
line4
line5`

	fileDiffs, err := diff.ParseMultiFileDiff([]byte(diffStr))
	require.NoError(t, err)
	require.Len(t, fileDiffs, 1)

	result, err := applyDiffToContent(original, fileDiffs[0])
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestApplyDiffToContent_ComplexMultiHunk(t *testing.T) {
	original := `import A
import B
import C

func main() {
	x := 1
	y := 2
	z := 3
	
	fmt.Println(x)
	fmt.Println(y)
	fmt.Println(z)
}`

	// Complex diff with multiple changes
	diffStr := `--- a/test.go
+++ b/test.go
@@ -1,5 +1,6 @@
 import A
 import B
+import D
 import C
 
 func main() {
@@ -7,7 +8,8 @@
 	y := 2
 	z := 3
 	
-	fmt.Println(x)
+	fmt.Println("x:", x)
 	fmt.Println(y)
-	fmt.Println(z)
+	fmt.Println("z:", z)
+	return
 }`

	expected := `import A
import B
import D
import C

func main() {
	x := 1
	y := 2
	z := 3
	
	fmt.Println("x:", x)
	fmt.Println(y)
	fmt.Println("z:", z)
	return
}`

	fileDiffs, err := diff.ParseMultiFileDiff([]byte(diffStr))
	require.NoError(t, err)
	require.Len(t, fileDiffs, 1)

	result, err := applyDiffToContent(original, fileDiffs[0])
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

