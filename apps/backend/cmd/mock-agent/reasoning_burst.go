package main

import (
	"fmt"
	"strings"
)

const (
	reasoningBurstDefaultCount = 1000
	reasoningBurstMaxCount     = 100_000
)

// parseReasoningBurstCommand parses the deterministic script command used by
// overload tests. The command emits no delay and keeps the count visible in a
// final marker message so an E2E test can prove the producer actually ran.
func parseReasoningBurstCommand(line string) (count int, ok bool) {
	for _, prefix := range []string{"e2e:reasoning_burst(", "e2e:reasoning-burst("} {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		count = extractIntArg(line, prefix)
		if count <= 0 {
			count = reasoningBurstDefaultCount
		}
		if count > reasoningBurstMaxCount {
			count = reasoningBurstMaxCount
		}
		return count, true
	}
	return 0, false
}

func reasoningBurstChunk(index int) string {
	return fmt.Sprintf("reasoning-burst-%06d|", index)
}

func reasoningBurstContent(count int) string {
	var content strings.Builder
	content.Grow(count * len("reasoning-burst-000000|"))
	for index := 1; index <= count; index++ {
		content.WriteString(reasoningBurstChunk(index))
	}
	return content.String()
}

func emitReasoningBurst(e *emitter, count int) {
	for index := 1; index <= count; index++ {
		e.thought(reasoningBurstChunk(index))
	}
	e.text(fmt.Sprintf("reasoning-burst-produced:%d", count))
}
