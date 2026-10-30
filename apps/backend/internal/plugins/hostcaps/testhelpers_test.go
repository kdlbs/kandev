package hostcaps

func setRegistryForTest(entries []Entry) func() {
	previous := registry
	registry = entries
	return func() { registry = previous }
}

func setRetiredForTest(pairs []RetiredPair) func() {
	previous := retired
	retired = pairs
	return func() { retired = previous }
}
