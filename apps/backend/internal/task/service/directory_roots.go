package service

func driveRootsFromMask(mask uint32) []DirectoryEntry {
	roots := make([]DirectoryEntry, 0, 26)
	for drive := 0; drive < 26; drive++ {
		if mask&(1<<drive) == 0 {
			continue
		}
		root := string(rune('A'+drive)) + `:\`
		roots = append(roots, DirectoryEntry{Name: root, Path: root})
	}
	return roots
}
