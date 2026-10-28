package metrics

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestSampler(read func(context.Context, string) (DiskCapacity, error)) *DiskCapacitySampler {
	sampler := NewDiskCapacitySampler()
	sampler.read = read
	sampler.identity = func(path string) (string, error) {
		if path == "" {
			return "", errors.New("missing identity")
		}
		return "device:" + path, nil
	}
	sampler.now = time.Now
	return sampler
}

func TestDeliveryStorageSamplerSharesOneReadAcrossConcurrentTargets(t *testing.T) {
	var reads atomic.Int64
	sampler := newTestSampler(func(context.Context, string) (DiskCapacity, error) {
		reads.Add(1)
		time.Sleep(10 * time.Millisecond)
		return DiskCapacity{TotalBytes: 1000, AvailableBytes: 100}, nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sample, err := sampler.Sample(context.Background(), "/workspace")
			if err != nil {
				t.Error(err)
				return
			}
			if sample.Capacity.TotalBytes != 1000 {
				t.Errorf("sample = %+v", sample)
			}
		}()
	}
	wg.Wait()
	if reads.Load() != 1 {
		t.Fatalf("capacity reads = %d, want 1 shared read", reads.Load())
	}
}

func TestDeliveryStorageSamplerKeepsSeparateFilesystemsSeparate(t *testing.T) {
	var reads atomic.Int64
	sampler := newTestSampler(func(context.Context, string) (DiskCapacity, error) {
		reads.Add(1)
		return DiskCapacity{TotalBytes: 1000, AvailableBytes: 500}, nil
	})
	if _, err := sampler.Sample(context.Background(), "/one"); err != nil {
		t.Fatal(err)
	}
	if _, err := sampler.Sample(context.Background(), "/two"); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 2 {
		t.Fatalf("capacity reads = %d, want one per filesystem", reads.Load())
	}
}

func TestDeliveryStorageSamplerReusesFreshSampleAndRefreshesStale(t *testing.T) {
	var reads atomic.Int64
	sampler := newTestSampler(func(context.Context, string) (DiskCapacity, error) {
		reads.Add(1)
		return DiskCapacity{TotalBytes: 1000, AvailableBytes: 500}, nil
	})
	sampler.SetTTL(time.Minute)
	now := time.Now()
	sampler.now = func() time.Time { return now }
	if _, err := sampler.Sample(context.Background(), "/workspace"); err != nil {
		t.Fatal(err)
	}
	if _, err := sampler.Sample(context.Background(), "/workspace"); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 1 {
		t.Fatalf("fresh sample was not reused: reads = %d", reads.Load())
	}
	now = now.Add(2 * time.Minute)
	if _, err := sampler.Sample(context.Background(), "/workspace"); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 2 {
		t.Fatalf("stale sample was not refreshed: reads = %d", reads.Load())
	}
}

func TestDeliveryStorageSamplerFailureDoesNotCacheHealthyState(t *testing.T) {
	var reads atomic.Int64
	sampler := newTestSampler(func(context.Context, string) (DiskCapacity, error) {
		if reads.Add(1) == 1 {
			return DiskCapacity{}, errors.New("statfs unavailable")
		}
		return DiskCapacity{TotalBytes: 1000, AvailableBytes: 900}, nil
	})
	if _, err := sampler.Sample(context.Background(), "/workspace"); err == nil {
		t.Fatal("failed sample was reported as healthy")
	}
	sample, err := sampler.Sample(context.Background(), "/workspace")
	if err != nil || sample.Capacity.AvailableBytes != 900 {
		t.Fatalf("retry sample = %+v, err = %v", sample, err)
	}
}

func TestDeliveryStorageSamplerBoundsIdentityAndCapacityDeadlines(t *testing.T) {
	release := make(chan struct{})
	sampler := newTestSampler(func(ctx context.Context, _ string) (DiskCapacity, error) {
		<-ctx.Done()
		return DiskCapacity{}, ctx.Err()
	})
	sampler.identity = func(string) (string, error) {
		<-release
		return "device:blocked", nil
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := sampler.Sample(ctx, "/blocked"); err == nil {
		t.Fatal("blocked identity lookup was reported as a healthy sample")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("sample exceeded its deadline: %s", elapsed)
	}
	close(release)
}
