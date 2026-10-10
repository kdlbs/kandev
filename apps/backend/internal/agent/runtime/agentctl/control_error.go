package client

// ControlHTTPError preserves the server status independently of its error text.
type ControlHTTPError struct {
	Status  int
	Message string
}

func (e *ControlHTTPError) Error() string       { return e.Message }
func (e *ControlHTTPError) HTTPStatusCode() int { return e.Status }
