package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	usermodels "github.com/kandev/kandev/internal/user/models"
	userservice "github.com/kandev/kandev/internal/user/service"
)

const (
	contentionEventInterval     = 10 * time.Millisecond
	contentionBatchSize         = 2
	contentionQueueSize         = 8192
	contentionAckCadence        = 20 * time.Millisecond
	contentionColorEveryCreates = 10
	contentionColorMinSamples   = 3
)

type contentionColorResult struct {
	started time.Time
	latency time.Duration
	color   string
	err     error
}

type contentionMeasurement struct {
	offered        int64
	projected      int64
	dropped        int64
	streamFailures int64
	ackFailures    int64
	latencies      []time.Duration
	colorResults   []contentionColorResult
	lastColor      string
	window         time.Duration
}

// BenchmarkTaskCreateContention measures complete HTTP task creation against
// production SQLite writer/reader pools while real durable agent-delivery
// streams and unrelated color edits contend for the same writer. Only the
// asynchronous provider launch is substituted.
func BenchmarkTaskCreateContention(b *testing.B) {
	b.Run("final", func(b *testing.B) {
		for _, startAgent := range []bool{true, false} {
			for _, streams := range []int{0, 4, 8} {
				name := fmt.Sprintf("start_agent=%t/sessions_%d", startAgent, streams)
				b.Run(name, func(b *testing.B) {
					runTaskCreateContention(b, startAgent, streams)
				})
			}
		}
	})
}

func runTaskCreateContention(b *testing.B, startAgent bool, streamCount int) {
	b.Helper()
	fixture := newTaskCreateContentionFixture(b)
	b.Cleanup(func() { _ = fixture.orchestrator.Stop() })

	for i := 0; i < 5; i++ {
		status, _, body := fixture.createTaskHTTP(startAgent)
		require.Equal(b, http.StatusOK, status, string(body))
	}

	targets := make([]*contentionDurableTarget, 0, streamCount)
	for i := 0; i < streamCount; i++ {
		targets = append(targets, fixture.seedDurableStream(i))
	}
	streamCtx, cancelStreams := context.WithCancel(context.Background())
	var streamWG sync.WaitGroup
	for _, target := range targets {
		streamWG.Add(2)
		go func(target *contentionDurableTarget) {
			defer streamWG.Done()
			runDurableStream(streamCtx, fixture, target)
		}(target)
		go func(target *contentionDurableTarget) {
			defer streamWG.Done()
			runDurableAckWorker(streamCtx, fixture.repo, target)
		}(target)
	}
	var stopStreamsOnce sync.Once
	stopStreams := func() {
		stopStreamsOnce.Do(func() {
			cancelStreams()
			streamWG.Wait()
		})
	}
	b.Cleanup(stopStreams)
	if streamCount > 0 {
		waitForDurableStreams(b, targets)
	}
	fixture.measurementEnd.Store(0)

	colorCtx, cancelColors := context.WithCancel(context.Background())
	colorTriggers := make(chan struct{}, 32)
	var colorWG sync.WaitGroup
	colorWG.Add(1)
	var colorMu sync.Mutex
	colorResults := make([]contentionColorResult, 0, 8)
	lastColor := ""
	go func() {
		defer colorWG.Done()
		sequence := 0
		for range colorTriggers {
			sequence++
			color := []string{"blue", "green", "red", "purple"}[sequence%4]
			started := time.Now()
			_, err := fixture.userService.UpdateUserSettings(colorCtx, &userservice.UpdateUserSettingsRequest{
				SidebarTaskColorPatch: &usermodels.SidebarTaskColorPatch{
					Colors: map[string]*string{fixture.unrelatedTaskID: &color},
				},
			})
			colorMu.Lock()
			colorResults = append(colorResults, contentionColorResult{
				started: started, latency: time.Since(started), color: color, err: err,
			})
			if err == nil {
				lastColor = color
			}
			colorMu.Unlock()
		}
	}()

	var stopColorOnce sync.Once
	stopColor := func() {
		stopColorOnce.Do(func() {
			close(colorTriggers)
			colorWG.Wait()
			cancelColors()
		})
	}
	b.Cleanup(stopColor)

	writerStatsBefore := fixture.writer.Stats()
	measurement := contentionMeasurement{latencies: make([]time.Duration, 0, b.N)}
	b.ResetTimer()
	measureStart := time.Now()
	for _, target := range targets {
		target.mu.Lock()
		target.startSequence.Store(target.offered.Load())
		target.projected.Store(0)
		target.dropped.Store(0)
		target.streamFailures.Store(0)
		target.ackFailures.Store(0)
		target.mu.Unlock()
	}
	for i := 0; i < b.N; i++ {
		if i%contentionColorEveryCreates == 0 {
			select {
			case colorTriggers <- struct{}{}:
			default:
			}
		}
		status, latency, body := fixture.createTaskHTTP(startAgent)
		if status != http.StatusOK {
			var payload map[string]interface{}
			_ = json.Unmarshal(body, &payload)
			b.Fatalf("create %d failed: status=%d body=%v", i, status, payload)
		}
		measurement.latencies = append(measurement.latencies, latency)
	}
	measureEnd := time.Now()
	b.StopTimer()
	fixture.measurementEnd.Store(measureEnd.UnixNano())
	writerStatsAfter := fixture.writer.Stats()
	measurement.window = measureEnd.Sub(measureStart)

	stopStreams()
	for _, target := range targets {
		measurement.offered += target.offered.Load() - target.startSequence.Load()
		measurement.projected += target.projected.Load()
		measurement.dropped += target.dropped.Load()
		measurement.streamFailures += target.streamFailures.Load()
		measurement.ackFailures += target.ackFailures.Load()
	}
	stopColor()

	colorMu.Lock()
	for _, result := range colorResults {
		if result.started.After(measureEnd) {
			continue
		}
		measurement.colorResults = append(measurement.colorResults, result)
	}
	measurement.lastColor = lastColor
	colorMu.Unlock()

	sort.Slice(measurement.latencies, func(i, j int) bool { return measurement.latencies[i] < measurement.latencies[j] })
	colorLatencies := make([]time.Duration, 0, len(measurement.colorResults))
	colorErrors := 0
	for _, result := range measurement.colorResults {
		if result.err != nil {
			colorErrors++
			continue
		}
		colorLatencies = append(colorLatencies, result.latency)
	}
	sort.Slice(colorLatencies, func(i, j int) bool { return colorLatencies[i] < colorLatencies[j] })

	windowSeconds := measurement.window.Seconds()
	if windowSeconds <= 0 {
		windowSeconds = 1
	}
	b.ReportMetric(float64(p95Milliseconds(measurement.latencies)), "create_p95_ms")
	b.ReportMetric(float64(p50Milliseconds(measurement.latencies)), "create_p50_ms")
	b.ReportMetric(float64(p99Milliseconds(measurement.latencies)), "create_p99_ms")
	b.ReportMetric(float64(colorErrors), "color_errors")
	b.ReportMetric(float64(len(measurement.colorResults)), "color_samples")
	b.ReportMetric(float64(p95Milliseconds(colorLatencies)), "color_p95_ms")
	b.ReportMetric(float64(measurement.offered), "offered_events")
	b.ReportMetric(float64(measurement.projected), "projected_events")
	b.ReportMetric(float64(measurement.dropped), "dropped_events")
	b.ReportMetric(float64(measurement.streamFailures), "stream_errors")
	b.ReportMetric(float64(measurement.ackFailures), "ack_errors")
	b.ReportMetric(float64(measurement.offered)/windowSeconds, "offered_events_per_s")
	b.ReportMetric(float64(measurement.projected)/windowSeconds, "projected_events_per_s")
	if measurement.offered > 0 {
		b.ReportMetric(float64(measurement.projected)/float64(measurement.offered)*100, "delivery_%")
	} else {
		b.ReportMetric(0, "delivery_%")
	}
	b.ReportMetric(float64(writerStatsAfter.WaitCount-writerStatsBefore.WaitCount), "writer_waits")
	b.ReportMetric(writerStatsAfter.WaitDuration.Seconds()-writerStatsBefore.WaitDuration.Seconds(), "writer_wait_s")

	if measurement.streamFailures > 0 {
		b.Fatalf("stream errors inside measurement: %d", measurement.streamFailures)
	}
	if measurement.ackFailures > 0 {
		b.Fatalf("durability barrier errors inside measurement: %d", measurement.ackFailures)
	}
	if colorErrors > 0 {
		b.Fatalf("color errors inside measurement: %d", colorErrors)
	}
	if b.N >= contentionColorEveryCreates*contentionColorMinSamples {
		validateContentionMeasurement(b, fixture, measurement, streamCount)
	}
}

// runDurableStream keeps offered events independent of writer completion,
// then admits and projects production-shaped batches. Events offered before
// the measurement window or committed after it do not count as delivered.
func runDurableStream(ctx context.Context, fixture *taskCreateContentionFixture, target *contentionDurableTarget) {
	events := make(chan *models.AgentDeliveryEvent, contentionQueueSize)
	go func() {
		defer close(events)
		ticker := time.NewTicker(contentionEventInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if end := fixture.measurementEnd.Load(); end > 0 && time.Now().UnixNano() > end {
					return
				}
				sequence := target.offered.Add(1)
				event := fixture.deliveryEvent(target, sequence)
				select {
				case events <- event:
				case <-ctx.Done():
					return
				default:
					target.dropped.Add(1)
				}
			}
		}
	}()

	var batch []*models.AgentDeliveryEvent
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			batch = append(batch, event)
			if len(batch) < contentionBatchSize {
				continue
			}
			if _, err := fixture.repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(
				ctx, batch, deliveryEffects(batch), batch[len(batch)-1].Sequence,
			); err != nil {
				if isDeliveryShutdown(ctx, err) {
					return
				}
				target.streamFailures.Add(1)
				batch = nil
				continue
			}
			committedAt := time.Now()
			end := fixture.measurementEnd.Load()
			if end == 0 || committedAt.UnixNano() <= end {
				target.mu.Lock()
				startSequence := target.startSequence.Load()
				for _, delivered := range batch {
					if delivered.Sequence > startSequence {
						target.projected.Add(1)
					}
				}
				target.mu.Unlock()
			}
			batch = nil
		}
	}
}

func runDurableAckWorker(ctx context.Context, repo interface {
	AwaitAgentDeliveryDurability(context.Context) error
}, target *contentionDurableTarget) {
	ticker := time.NewTicker(contentionAckCadence)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := repo.AwaitAgentDeliveryDurability(ctx); err != nil {
				if isDeliveryShutdown(ctx, err) {
					return
				}
				target.ackFailures.Add(1)
				return
			}
		}
	}
}

func waitForDurableStreams(tb testing.TB, targets []*contentionDurableTarget) {
	tb.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		ready := true
		for _, target := range targets {
			if target.projected.Load() < contentionBatchSize || target.offered.Load() < contentionBatchSize {
				ready = false
				break
			}
		}
		if ready {
			return
		}
		if time.Now().After(deadline) {
			tb.Fatalf("durable streams did not project an initial batch within 10s")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func validateContentionMeasurement(
	b *testing.B,
	fixture *taskCreateContentionFixture,
	measurement contentionMeasurement,
	streamCount int,
) {
	b.Helper()
	if len(measurement.colorResults) < contentionColorMinSamples {
		b.Fatalf("color samples = %d, want at least %d", len(measurement.colorResults), contentionColorMinSamples)
	}
	if streamCount > 0 {
		if measurement.offered == 0 {
			b.Fatal("durable streams offered no events")
		}
		expectedOffers := float64(streamCount) * measurement.window.Seconds() / contentionEventInterval.Seconds()
		if float64(measurement.offered) < expectedOffers*0.9 {
			b.Fatalf("offered events = %d, want at least 90%% of %.0f", measurement.offered, expectedOffers)
		}
		if measurement.projected*100 < measurement.offered*90 {
			b.Fatalf("timely durable delivery = %d/%d, want >=90%%", measurement.projected, measurement.offered)
		}
	}
	if measurement.lastColor == "" {
		b.Fatal("no unrelated color was saved")
	}
	settings, err := fixture.userService.GetUserSettings(context.Background())
	require.NoError(b, err)
	saved := settings.SidebarTaskColors[fixture.unrelatedTaskID]
	if saved == nil || *saved != measurement.lastColor {
		b.Fatalf("unrelated color readback = %v, want %q", saved, measurement.lastColor)
	}
}

func p50Milliseconds(latencies []time.Duration) float64 {
	return percentileMilliseconds(latencies, 0.50)
}

func p95Milliseconds(latencies []time.Duration) float64 {
	return percentileMilliseconds(latencies, 0.95)
}

func p99Milliseconds(latencies []time.Duration) float64 {
	return percentileMilliseconds(latencies, 0.99)
}

func percentileMilliseconds(latencies []time.Duration, percentile float64) float64 {
	if len(latencies) == 0 {
		return 0
	}
	index := int(float64(len(latencies)-1) * percentile)
	return float64(latencies[index]) / float64(time.Millisecond)
}
