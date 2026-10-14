package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeliveryRetentionConfig(t *testing.T) {
	t.Run("defaults and YAML provenance", func(t *testing.T) {
		cfg, err := LoadWithPath(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		got := cfg.Tasks.DeliveryRetention
		if got.PayloadGrace != time.Hour || got.ReplayAge != 24*time.Hour || got.ReplayEvents != 10000 || got.Interval != time.Minute || got.BatchRows != 200 || got.BatchBytes != 4<<20 {
			t.Fatalf("defaults = %+v", got)
		}

		configFile := filepath.Join(t.TempDir(), "config.yaml")
		contents := "tasks:\n  deliveryRetention:\n    payloadGrace: 2h\n    replayAge: 48h\n    replayEvents: 12000\n    interval: 2m\n    batchRows: 150\n    batchBytes: 3145728\n"
		if err := os.WriteFile(configFile, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err = LoadWithPath(configFile)
		if err != nil {
			t.Fatal(err)
		}
		got = cfg.Tasks.DeliveryRetention
		if got.PayloadGrace != 2*time.Hour || got.ReplayAge != 48*time.Hour || got.ReplayEvents != 12000 || got.Interval != 2*time.Minute || got.BatchRows != 150 || got.BatchBytes != 3145728 {
			t.Fatalf("YAML values = %+v", got)
		}
		for _, key := range deliveryRetentionKeys {
			if source := cfg.SourceFor(key); source != SourceConfiguration {
				t.Errorf("source for %s = %q", key, source)
			}
		}
	})

	t.Run("environment aliases override YAML", func(t *testing.T) {
		t.Setenv("KANDEV_TASK_DELIVERY_RETENTION_PAYLOAD_GRACE", "3h")
		t.Setenv("KANDEV_TASK_DELIVERY_RETENTION_REPLAY_AGE", "72h")
		t.Setenv("KANDEV_TASK_DELIVERY_RETENTION_REPLAY_EVENTS", "13000")
		t.Setenv("KANDEV_TASK_DELIVERY_RETENTION_INTERVAL", "3m")
		t.Setenv("KANDEV_TASK_DELIVERY_RETENTION_BATCH_ROWS", "175")
		t.Setenv("KANDEV_TASK_DELIVERY_RETENTION_BATCH_BYTES", "2097152")
		cfg, err := LoadWithPath(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		got := cfg.Tasks.DeliveryRetention
		if got.PayloadGrace != 3*time.Hour || got.ReplayAge != 72*time.Hour || got.ReplayEvents != 13000 || got.Interval != 3*time.Minute || got.BatchRows != 175 || got.BatchBytes != 2097152 {
			t.Fatalf("environment values = %+v", got)
		}
		for _, key := range deliveryRetentionKeys {
			if source := cfg.SourceFor(key); source != SourceEnvironment {
				t.Errorf("source for %s = %q", key, source)
			}
		}
	})

	t.Run("invalid YAML and environment values fail startup", func(t *testing.T) {
		for _, tc := range []struct{ name, yaml string }{
			{"payload grace", "payloadGrace: 30s"},
			{"replay age", "replayAge: 31d"},
			{"replay events", "replayEvents: 99"},
			{"interval", "interval: 5s"},
			{"rows", "batchRows: 201"},
			{"bytes", "batchBytes: 512"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "config.yaml")
				if err := os.WriteFile(path, []byte("tasks:\n  deliveryRetention:\n    "+tc.yaml+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := LoadWithPath(path); err == nil {
					t.Fatalf("LoadWithPath accepted %s", tc.yaml)
				}
			})
		}
		t.Setenv("KANDEV_TASK_DELIVERY_RETENTION_BATCH_ROWS", "0")
		if _, err := LoadWithPath(t.TempDir()); err == nil {
			t.Fatal("LoadWithPath accepted an invalid environment override")
		}
	})
}

var deliveryRetentionKeys = []string{
	"tasks.deliveryRetention.payloadGrace",
	"tasks.deliveryRetention.replayAge",
	"tasks.deliveryRetention.replayEvents",
	"tasks.deliveryRetention.interval",
	"tasks.deliveryRetention.batchRows",
	"tasks.deliveryRetention.batchBytes",
}
