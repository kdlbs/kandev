package sqlite

import (
	"expvar"
	"strconv"
	"time"
)

var (
	messagePayloadHydrationsTotal = expvar.NewMap("task_message_payload_hydrations_total")
	messagePayloadHydrationMS     = expvar.NewMap("task_message_payload_hydration_latency_ms")
)

var hydrationBuckets = [...]int64{10, 50, 250, 1_000}

func recordMessagePayloadHydration(elapsed time.Duration, err error) {
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	messagePayloadHydrationsTotal.Add("outcome="+outcome, 1)
	millis := elapsed.Milliseconds()
	for _, upperBound := range hydrationBuckets {
		if millis <= upperBound {
			messagePayloadHydrationMS.Add("outcome="+outcome+";le="+strconv.FormatInt(upperBound, 10), 1)
		}
	}
	messagePayloadHydrationMS.Add("outcome="+outcome+";le=+Inf", 1)
}
