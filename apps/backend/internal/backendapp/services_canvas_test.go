package backendapp

import "testing"

func TestInitCanvasServiceFailsClosedWhenEnabledWithoutPlugins(t *testing.T) {
	service, err := initCanvasService(true, nil, nil, nil, nil, nil)
	if service != nil {
		t.Fatal("expected no canvas service when initialization fails")
	}
	if err == nil {
		t.Fatal("expected enabled canvas initialization to fail without the plugin service")
	}
}

func TestInitCanvasServiceSkipsDisabledFeature(t *testing.T) {
	service, err := initCanvasService(false, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("disabled canvas initialization: %v", err)
	}
	if service != nil {
		t.Fatal("expected disabled canvas initialization to return no service")
	}
}
