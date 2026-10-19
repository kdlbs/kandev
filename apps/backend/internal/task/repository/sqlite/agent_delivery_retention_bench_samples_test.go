package sqlite

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func writeRetentionBenchmarkSamples(b *testing.B, samples []time.Duration) {
	b.Helper()
	directory := os.Getenv("KANDEV_RETENTION_BENCH_SAMPLES_DIR")
	if directory == "" {
		return
	}
	nanoseconds := make([]int64, len(samples))
	for index, sample := range samples {
		nanoseconds[index] = sample.Nanoseconds()
	}
	file, err := os.CreateTemp(directory, strings.ReplaceAll(b.Name(), "/", "-")+"-*.json")
	if err != nil {
		b.Fatal(err)
	}
	data := struct {
		Name        string  `json:"name"`
		Iterations  int     `json:"iterations"`
		Nanoseconds []int64 `json:"nanoseconds"`
	}{Name: b.Name(), Iterations: b.N, Nanoseconds: nanoseconds}
	if err := json.NewEncoder(file).Encode(data); err != nil {
		_ = file.Close()
		b.Fatal(err)
	}
	if err := file.Close(); err != nil {
		b.Fatal(err)
	}
}
