package storage

import (
	"expvar"
	"strconv"
	"time"
)

// Storage analysis metrics are exposed through the stdlib /debug/vars
// endpoint in development mode. Duration maps use one bucket per observation;
// this keeps the expvar contract useful without introducing a second metrics
// registry for a process-local cache.
var (
	storageAnalysisScansStartedTotal         = expvar.NewInt("storage_analysis_scans_started_total")
	storageAnalysisScansJoinedTotal          = expvar.NewInt("storage_analysis_scans_joined_total")
	storageAnalysisScansCompletedTotal       = expvar.NewInt("storage_analysis_scans_completed_total")
	storageAnalysisScansFailedTotal          = expvar.NewInt("storage_analysis_scans_failed_total")
	storageAnalysisDurationBucketTotal       = expvar.NewMap("storage_analysis_duration_bucket_total")
	storageAnalysisSourceDurationBucketTotal = expvar.NewMap(
		"storage_analysis_source_duration_bucket_total",
	)
)

func recordStorageAnalysisStarted() {
	storageAnalysisScansStartedTotal.Add(1)
}

func recordStorageAnalysisJoined() {
	storageAnalysisScansJoinedTotal.Add(1)
}

func recordStorageAnalysisCompleted(completion StorageAnalysisCompletion) {
	storageAnalysisScansCompletedTotal.Add(1)
	if !completion.Succeeded {
		storageAnalysisScansFailedTotal.Add(1)
	}
	storageAnalysisDurationBucketTotal.Add(durationBucket(completion.Duration), 1)
	for source, duration := range completion.SourceDurations {
		storageAnalysisSourceDurationBucketTotal.Add(
			"source="+source+";bucket="+durationBucket(duration), 1,
		)
	}
}

func durationBucket(duration time.Duration) string {
	milliseconds := duration.Milliseconds()
	for _, boundary := range []int64{10, 50, 100, 500, 1000, 5000, 30000, 60000} {
		if milliseconds <= boundary {
			return "le_" + strconv.FormatInt(boundary, 10) + "ms"
		}
	}
	return "gt_60000ms"
}
