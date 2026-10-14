package lifecycle

import "testing"

// TestRemoteDockerReconnectDelegateCarriesBrokerPreflight keeps a broker-backed
// resume from calling a nil preflight. The delegate runs findExistingInstance,
// which preflights the credential broker before replacing a stale instance.
func TestRemoteDockerReconnectDelegateCarriesBrokerPreflight(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))

	delegate := exec.reconnectDelegate(&remoteDockerSession{})

	if delegate.brokerPreflight == nil {
		t.Fatal("reconnect delegate has no broker preflight; a broker-backed resume would panic")
	}
}
