package acp

func newMockACPDialect() acpDialect {
	return acpDialect{responseAttemptReset: mockResponseAttemptResetMeta}
}

func mockResponseAttemptResetMeta(meta map[string]any) bool {
	mock, ok := nestedMap(meta, "kandevMock")
	if !ok {
		return false
	}
	reset, ok := mock["responseAttemptReset"].(bool)
	return ok && reset
}
