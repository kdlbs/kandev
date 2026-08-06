import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { TaskLspLanguageSnapshot } from "@/lib/types/http-lsp";

const NOW = "2026-08-05T10:00:00Z";

function language(revision: number, phase: TaskLspLanguageSnapshot["phase"] = "ready") {
  return {
    task_id: "task-1",
    language: "kotlin",
    policy: "keep_warm",
    detected: true,
    detection_state: "complete",
    detection_truncated: false,
    phase,
    generation: 3,
    revision,
    last_transition_at: NOW,
    last_action: "start",
    last_initiator: "user",
    restart_required: false,
    created_at: NOW,
    updated_at: NOW,
    effective_policy: "keep_warm",
    activity: "idle",
    progress: [],
  } satisfies TaskLspLanguageSnapshot;
}

function store() {
  return createAppStore();
}

describe("LSP slice", () => {
  it("normalizes snapshots by task and language", () => {
    const subject = store();
    subject.getState().setTaskLspSnapshot({
      task_id: "task-1",
      languages: [language(4)],
      capacity: { active: 1, queued: 0, limit: 4 },
    });
    expect(subject.getState().taskLsp.byTaskId["task-1"]?.languages.kotlin?.revision).toBe(4);
    expect(subject.getState().taskLsp.byTaskId["task-1"]?.loaded).toBe(true);
  });

  it("rejects stale REST snapshots after a live update", () => {
    const subject = store();
    subject.getState().mergeTaskLspLanguage(language(9, "ready"));
    subject.getState().setTaskLspSnapshot({
      task_id: "task-1",
      languages: [language(8, "starting")],
      capacity: { active: 0, queued: 0, limit: 4 },
    });
    expect(subject.getState().taskLsp.byTaskId["task-1"]?.languages.kotlin?.phase).toBe("ready");
  });

  it("accepts same-revision runtime evidence and isolates tasks", () => {
    const subject = store();
    subject.getState().mergeTaskLspLanguage(language(9));
    subject.getState().mergeTaskLspLanguage({
      ...language(9),
      activity: "server_work",
      progress: [{ token: "gradle", title: "Importing", started_at: NOW }],
    });
    subject.getState().mergeTaskLspLanguage({ ...language(1), task_id: "task-2" });
    expect(subject.getState().taskLsp.byTaskId["task-1"]?.languages.kotlin?.activity).toBe(
      "server_work",
    );
    expect(subject.getState().taskLsp.byTaskId["task-2"]?.languages.kotlin?.revision).toBe(1);
  });
});
