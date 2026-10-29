package sandbox

// StartFailure carries private evidence between local execution-host owners.
// Its public text never includes runtime paths or receipt contents.
type StartFailure struct {
	Receipt *StartReceipt `json:"-"`
}

func (*StartFailure) Error() string { return "sandbox process failed to start" }
