package acp

import "testing"

func TestACPNotifQueueCapacity(t *testing.T) {
	t.Run("default uses max capacity for long session replays", func(t *testing.T) {
		t.Setenv("KANDEV_ACP_NOTIF_QUEUE", "")

		if got := acpNotifQueueCapacity(); got != acpNotifQueueDefault {
			t.Fatalf("default queue capacity = %d, want %d", got, acpNotifQueueDefault)
		}
		if got := acpNotifQueueDefault; got != acpNotifQueueMax {
			t.Fatalf("default queue capacity = %d, want max %d", got, acpNotifQueueMax)
		}
	})

	t.Run("invalid env falls back to default", func(t *testing.T) {
		t.Setenv("KANDEV_ACP_NOTIF_QUEUE", "not-a-number")

		if got := acpNotifQueueCapacity(); got != acpNotifQueueDefault {
			t.Fatalf("invalid env queue capacity = %d, want %d", got, acpNotifQueueDefault)
		}
	})

	t.Run("non-positive env falls back to default", func(t *testing.T) {
		for _, value := range []string{"0", "-5"} {
			t.Run(value, func(t *testing.T) {
				t.Setenv("KANDEV_ACP_NOTIF_QUEUE", value)

				if got := acpNotifQueueCapacity(); got != acpNotifQueueDefault {
					t.Fatalf("non-positive env queue capacity = %d, want %d", got, acpNotifQueueDefault)
				}
			})
		}
	})

	t.Run("in-range env is honored", func(t *testing.T) {
		t.Setenv("KANDEV_ACP_NOTIF_QUEUE", "4096")

		if got := acpNotifQueueCapacity(); got != 4096 {
			t.Fatalf("in-range env queue capacity = %d, want 4096", got)
		}
	})

	t.Run("env is clamped", func(t *testing.T) {
		t.Setenv("KANDEV_ACP_NOTIF_QUEUE", "1")
		if got := acpNotifQueueCapacity(); got != acpNotifQueueMin {
			t.Fatalf("low env queue capacity = %d, want %d", got, acpNotifQueueMin)
		}

		t.Setenv("KANDEV_ACP_NOTIF_QUEUE", "999999")
		if got := acpNotifQueueCapacity(); got != acpNotifQueueMax {
			t.Fatalf("high env queue capacity = %d, want %d", got, acpNotifQueueMax)
		}
	})
}
