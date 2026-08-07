import { describe, expect, it } from "vitest";
import type { TaskLspLanguageSnapshot } from "@/lib/types/http-lsp";
import { deriveTaskLspViewModel } from "./task-lsp-view-model";

const NOW = Date.parse("2026-08-05T12:05:00Z");
const TEST_TIME = "2026-08-05T12:00:00Z";

function language(
  id: string,
  overrides: Partial<TaskLspLanguageSnapshot> = {},
): TaskLspLanguageSnapshot {
  return {
    task_id: "task-1",
    language: id,
    policy: "inherit",
    detected: false,
    detection_state: "complete",
    detection_truncated: false,
    phase: "off",
    generation: 0,
    revision: 1,
    last_transition_at: TEST_TIME,
    last_action: "",
    last_initiator: "automatic",
    restart_required: false,
    created_at: TEST_TIME,
    updated_at: TEST_TIME,
    effective_policy: "inherit",
    activity: "idle",
    progress: [],
    ...overrides,
  };
}

describe("deriveTaskLspViewModel", () => {
  it("keeps every supported row available but filters and priority-orders the aggregate", () => {
    const view = deriveTaskLspViewModel(
      [
        language("python"),
        language("rust", { policy: "keep_warm", effective_policy: "keep_warm" }),
        language("go", { detected: true }),
        language("kotlin", {
          detected: true,
          phase: "error",
          generation: 2,
          error_code: "server_crashed",
        }),
      ],
      NOW,
    );

    expect(view.rows.map((row) => row.language)).toEqual(["go", "kotlin", "python", "rust"]);
    expect(view.relevantRows.map((row) => row.language)).toEqual(["kotlin", "go", "rust"]);
    expect(view.relevantRows.map((row) => row.state)).toEqual(["error", "detected", "configured"]);
    expect(view.aggregate.errorCount).toBe(1);
  });

  it("summarizes one server-reported work item with honest elapsed evidence", () => {
    const view = deriveTaskLspViewModel(
      [
        language("kotlin", {
          detected: true,
          phase: "initializing",
          generation: 1,
          process_started_at: TEST_TIME,
          initialize_started_at: "2026-08-05T12:00:15Z",
          activity: "server_work",
          progress: [
            {
              token: "gradle-import",
              title: "Importing Kotlin project",
              message: "Resolving Gradle model",
              started_at: "2026-08-05T12:01:00Z",
            },
          ],
        }),
      ],
      NOW,
    );

    expect(view.aggregate.compact).toEqual({
      kind: "single",
      language: "Kotlin",
      state: "server_work",
      workTitle: "Importing Kotlin project",
      elapsedMs: 240_000,
    });
    expect(view.rows[0].elapsedMs).toBe(240_000);
  });

  it("uses aggregate counts for multiple live languages and prioritizes errors", () => {
    const running = deriveTaskLspViewModel(
      [
        language("go", { detected: true, phase: "ready", generation: 1 }),
        language("rust", { detected: true, phase: "initializing", generation: 1 }),
      ],
      NOW,
    );
    expect(running.aggregate.compact).toEqual({
      kind: "multiple",
      runningCount: 2,
      errorCount: 0,
      workCount: 0,
      relevantCount: 2,
    });

    const errored = deriveTaskLspViewModel(
      [
        language("go", { detected: true, phase: "ready", generation: 1 }),
        language("rust", { detected: true, phase: "error", generation: 3 }),
      ],
      NOW,
    );
    expect(errored.aggregate.compact).toMatchObject({ kind: "multiple", errorCount: 1 });
    expect(errored.relevantRows[0].state).toBe("error");
  });
});

describe("deriveTaskLspViewModel actions and visibility", () => {
  it("derives safe Start, Stop, and Restart availability from authoritative phase", () => {
    const view = deriveTaskLspViewModel(
      [
        language("go", { detected: true }),
        language("kotlin", { detected: true, phase: "ready", generation: 1 }),
        language("python", { detected: true, phase: "stopping", generation: 1 }),
        language("rust", { detected: true, phase: "unsupported" }),
      ],
      NOW,
    );
    const byLanguage = Object.fromEntries(view.rows.map((row) => [row.language, row.actions]));

    expect(byLanguage.go).toEqual({ start: true, stop: true, restart: false });
    expect(byLanguage.kotlin).toEqual({ start: false, stop: true, restart: true });
    expect(byLanguage.python).toEqual({ start: false, stop: false, restart: false });
    expect(byLanguage.rust).toEqual({ start: false, stop: false, restart: false });
  });

  it("filters hidden languages from rows and aggregate counts without changing snapshots", () => {
    const go = language("go", { detected: true, phase: "ready", generation: 2 });
    const view = deriveTaskLspViewModel(
      [go, language("kotlin", { detected: true, phase: "ready", generation: 4 })],
      NOW,
      { hiddenLanguages: ["go"] },
    );

    expect(view.rows.map((row) => row.language)).toEqual(["kotlin"]);
    expect(view.relevantRows.map((row) => row.language)).toEqual(["kotlin"]);
    expect(view.aggregate.runningCount).toBe(1);
    expect(view.hiddenCount).toBe(1);
    expect(go.phase).toBe("ready");
    expect(go.generation).toBe(2);
  });

  it("force-shows a hidden current editor language and reports an all-hidden aggregate", () => {
    const languages = [language("go", { detected: true }), language("kotlin", { detected: true })];

    const hidden = deriveTaskLspViewModel(languages, NOW, {
      hiddenLanguages: ["go", "kotlin"],
    });
    expect(hidden.rows).toEqual([]);
    expect(hidden.relevantRows).toEqual([]);
    expect(hidden.hiddenCount).toBe(2);
    expect(hidden.aggregate.compact).toEqual({ kind: "empty" });

    const focused = deriveTaskLspViewModel(languages, NOW, {
      hiddenLanguages: ["go", "kotlin"],
      forceVisibleLanguage: "go",
    });
    expect(focused.rows.map((row) => row.language)).toEqual(["go"]);
    expect(focused.relevantRows.map((row) => row.language)).toEqual(["go"]);
    expect(focused.hiddenCount).toBe(2);
  });
});
