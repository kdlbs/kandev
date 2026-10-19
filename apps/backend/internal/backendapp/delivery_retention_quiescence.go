package backendapp

func quiesceDeliveryRetention(stop func(), next func() error) func() error {
	return func() error {
		if stop != nil {
			stop()
		}
		if next != nil {
			return next()
		}
		return nil
	}
}
