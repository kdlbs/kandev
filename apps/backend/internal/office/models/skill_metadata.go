package models

import (
	"crypto/sha256"
	"encoding/hex"
)

// SkillPackageContentHash returns the identity of the content and package
// metadata that determine a materialized skill package.
func SkillPackageContentHash(content, fileInventory, sourceLocator string) string {
	sum := sha256.Sum256([]byte(content + "\x00" + fileInventory + "\x00" + sourceLocator))
	return hex.EncodeToString(sum[:])
}
