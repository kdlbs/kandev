package client

// InferenceClientRef is an acquired native client and its server-owned locator.
// Release is required even when observation fails; it never settles an attempt.
type InferenceClientRef struct {
	Client   *Client
	Identity InferenceIdentity
	Release  func()
}

func (c *Client) InferenceInstanceID() string { return c.executionID }
