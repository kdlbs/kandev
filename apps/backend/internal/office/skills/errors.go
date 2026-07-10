package skills

import "errors"

// ErrSkillNotFound is returned by GetSkill when no skill row matches the
// supplied id or slug. Callers should classify with errors.Is so HTTP layers
// can translate to 404 without string-matching the formatted message.
var ErrSkillNotFound = errors.New("skill not found")

// ErrSkillFileNotFound is returned by file accessors (readLocalSkillFile,
// readUserHomeSkillInventoryFile, GetSkillFile) when the requested file is
// missing from the skill's source. Callers translate to 404.
var ErrSkillFileNotFound = errors.New("skill file not found")
