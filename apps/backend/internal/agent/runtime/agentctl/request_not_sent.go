package client

// RequestNotSentError proves that the transport did not attempt a request write.
// A write failure or a missing response cannot supply this evidence.
type RequestNotSentError struct{ Err error }

func (e *RequestNotSentError) Error() string { return e.Err.Error() }
func (e *RequestNotSentError) Unwrap() error { return e.Err }
