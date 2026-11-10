package models

// PreparedSessionSeed carries the initial prompt state that must be committed
// with a newly prepared session. Callers build and validate it before the
// session insert; reused sessions keep their existing metadata.
type PreparedSessionSeed struct {
	InitialPromptPreview    *InitialPromptPreview
	InitialPromptSubmission *InitialPromptSubmission
}

// Validate rejects malformed preview or submission state before any session
// row is written.
func (seed *PreparedSessionSeed) Validate() error {
	if seed == nil {
		return nil
	}
	if preview := seed.InitialPromptPreview; preview != nil &&
		(len(preview.Content) > MaxInitialPromptSubmissionBytes || len(preview.Attachments) > MaxMessageAttachmentCount) {
		return ErrInvalidInitialPromptSubmission
	}
	if seed.InitialPromptSubmission == nil {
		return nil
	}
	return seed.InitialPromptSubmission.Validate()
}

// Empty reports whether the seed carries no initial prompt state.
func (seed *PreparedSessionSeed) Empty() bool {
	return seed == nil || (seed.InitialPromptPreview == nil && seed.InitialPromptSubmission == nil)
}

// ApplyToMetadata writes the seed values into a session metadata map. It
// returns the map so callers can use it when metadata was nil.
func (seed *PreparedSessionSeed) ApplyToMetadata(metadata map[string]interface{}) map[string]interface{} {
	if seed.Empty() {
		return metadata
	}
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	if seed.InitialPromptPreview != nil {
		metadata[SessionMetaKeyInitialPromptPreview] = seed.InitialPromptPreview
	}
	if seed.InitialPromptSubmission != nil {
		metadata[SessionMetaKeyInitialPromptSubmission] = seed.InitialPromptSubmission
	}
	return metadata
}
