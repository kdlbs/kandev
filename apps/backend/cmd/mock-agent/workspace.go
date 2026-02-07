package main

import (
	"bufio"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// workspaceFiles holds discovered text files from the working directory.
// Populated once on first use via discoverFiles().
var workspaceFiles []fileInfo

type fileInfo struct {
	absPath string // absolute path
	relPath string // relative to working directory
}

// textExtensions are file extensions considered "text files" for mock operations.
var textExtensions = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".py": true, ".rs": true, ".java": true, ".c": true, ".h": true,
	".css": true, ".html": true, ".json": true, ".yaml": true, ".yml": true,
	".toml": true, ".md": true, ".txt": true, ".sh": true, ".sql": true,
	".graphql": true, ".proto": true, ".xml": true, ".svg": true,
	".env": true, ".gitignore": true, ".dockerignore": true,
}

// skipDirs are directories to skip during file discovery.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".next": true,
	"dist": true, "build": true, "bin": true, "__pycache__": true,
	".cache": true, ".turbo": true, "coverage": true,
}

const maxFiles = 200

// discoverFiles walks the working directory and collects text files.
func discoverFiles() []fileInfo {
	if workspaceFiles != nil {
		return workspaceFiles
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil
	}

	var files []fileInfo
	_ = filepath.Walk(wd, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) >= maxFiles {
			return filepath.SkipAll
		}
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if !textExtensions[ext] && !textExtensions[info.Name()] {
			return nil
		}
		// Skip very large files
		if info.Size() > 100*1024 {
			return nil
		}
		rel, _ := filepath.Rel(wd, path)
		files = append(files, fileInfo{absPath: path, relPath: rel})
		return nil
	})

	workspaceFiles = files
	return workspaceFiles
}

// randomFile returns a random file from the workspace, or a fallback.
func randomFile() fileInfo {
	files := discoverFiles()
	if len(files) == 0 {
		return fileInfo{absPath: "/workspace/example.txt", relPath: "example.txt"}
	}
	return files[rand.Intn(len(files))]
}

// randomFileExcluding returns a random file that isn't in the exclude set.
func randomFileExcluding(exclude map[string]bool) fileInfo {
	files := discoverFiles()
	if len(files) == 0 {
		return fileInfo{absPath: "/workspace/example.txt", relPath: "example.txt"}
	}
	// Try up to 20 times to find a non-excluded file
	for range 20 {
		f := files[rand.Intn(len(files))]
		if !exclude[f.absPath] {
			return f
		}
	}
	return files[rand.Intn(len(files))]
}

// readFileSnippet reads up to maxLines lines from a file.
func readFileSnippet(path string, maxLines int) string {
	f, err := os.Open(path)
	if err != nil {
		return "// (file not readable)\n"
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	var lines []string
	for scanner.Scan() && len(lines) < maxLines {
		lines = append(lines, scanner.Text())
	}
	return strings.Join(lines, "\n") + "\n"
}

// pickEditableFragment finds a line in the file suitable for a mock edit.
// Returns (oldString, newString) where newString has a word replaced.
func pickEditableFragment(path string) (old, new_ string) {
	f, err := os.Open(path)
	if err != nil {
		return "hello", "hello_mock"
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	var candidates []string
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		// Pick lines that are non-empty, not too short, and look like code
		if len(trimmed) >= 10 && len(trimmed) <= 120 && utf8.ValidString(trimmed) {
			candidates = append(candidates, line)
		}
	}

	if len(candidates) == 0 {
		return "original", "modified"
	}

	line := candidates[rand.Intn(len(candidates))]
	// Find a word to replace
	words := strings.Fields(line)
	if len(words) == 0 {
		return line, line + " // mock-edited"
	}
	// Pick a non-trivial word (length > 2)
	var editableWords []int
	for i, w := range words {
		if len(w) > 2 {
			editableWords = append(editableWords, i)
		}
	}
	if len(editableWords) == 0 {
		return line, line + " // mock-edited"
	}
	idx := editableWords[rand.Intn(len(editableWords))]
	newWords := make([]string, len(words))
	copy(newWords, words)
	newWords[idx] = words[idx] + "_mock"
	return line, strings.Join(newWords, " ")
}

// randomFilePaths returns n random file relative paths for search results.
func randomFilePaths(n int) []string {
	files := discoverFiles()
	if len(files) == 0 {
		return []string{"example.txt"}
	}
	if n > len(files) {
		n = len(files)
	}
	// Shuffle and take first n
	perm := rand.Perm(len(files))
	paths := make([]string, n)
	for i := 0; i < n; i++ {
		paths[i] = files[perm[i]].relPath
	}
	return paths
}
