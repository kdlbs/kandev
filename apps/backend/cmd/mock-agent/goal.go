package main

import (
	"strings"
	"time"
)

const goalObjective = "Coordinate contributor PR reviews and keep the review queue current. " +
	"Review the outstanding changes, record the relevant decisions, and continue with the next " +
	"review when the current one is complete."

func goalSnapshot(status string, createdAt, updatedAt int64) map[string]any {
	return map[string]any{
		"objective":       goalObjective,
		"status":          status,
		"createdAt":       createdAt,
		"updatedAt":       updatedAt,
		"tokenBudget":     nil,
		"tokensUsed":      42,
		"timeUsedSeconds": 3,
		"controlMethod":   "_session/goal",
	}
}

func emitGoalActive(e *emitter, createdAt, updatedAt int64) {
	e.sessionInfo(map[string]any{"goal": goalSnapshot("active", createdAt, updatedAt)})
	waitForDelay(e.ctx, 250)
	e.sessionInfo(map[string]any{
		"codex": map[string]any{"threadStatus": "idle"},
	})
}

func scenarioGoalActive(e *emitter) {
	now := time.Now().UnixMilli()
	emitGoalActive(e, now, now)
	e.text("The provider goal remains active after the thread becomes idle.")
}

func scenarioGoalComplete(e *emitter) {
	now := time.Now().UnixMilli()
	emitGoalActive(e, now, now)
	waitForDelay(e.ctx, 400)
	e.sessionInfo(map[string]any{"goal": goalSnapshot("complete", now, now+1)})
	e.text("The provider goal is complete.")
}

func scenarioGoalClear(e *emitter) {
	now := time.Now().UnixMilli()
	emitGoalActive(e, now, now)
	waitForDelay(e.ctx, 400)
	e.sessionInfo(map[string]any{"goal": nil})
	e.text("The provider goal was cleared.")
}

func scenarioGoalLong(e *emitter) {
	now := time.Now().UnixMilli()
	longObjective := goalObjective + " " + strings.Repeat("Keep the review context available. ", 80)
	e.sessionInfo(map[string]any{
		"goal": map[string]any{
			"objective":     longObjective,
			"status":        "active",
			"createdAt":     now,
			"updatedAt":     now,
			"controlMethod": "_session/goal",
		},
	})
	e.text("The provider reported a long active goal.")
}
