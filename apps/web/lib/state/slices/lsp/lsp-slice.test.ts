import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { TaskLspCapacity, TaskLspLanguageSnapshot } from "@/lib/types/http-lsp";

const NOW = "2026-08-05T10:00:00Z";
const CAPACITY_EPOCH = "20260806T090000.000000002Z";
const PRIOR_CAPACITY_EPOCH = "20260806T090000.000000001Z";
const RESTARTED_CAPACITY_EPOCH = "20260806T080000.000000001Z";
const STOP_FAILED = "stop failed";

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

function expectSequencedCapacityAfterUnsequencedSnapshot() {
  const subject = store();
  subject.getState().setTaskLspSnapshot({
    task_id: "task-1",
    languages: [language(8, "starting")],
    capacity: { active: 1, queued: 0, limit: 4, revision: 99 },
  });
  subject.getState().mergeTaskLspLanguage({
    ...language(9, "ready"),
    capacity: {
      active: 0,
      queued: 0,
      limit: 4,
      revision: 1,
      epoch: CAPACITY_EPOCH,
    },
  });
  expect(subject.getState().taskLsp.byTaskId["task-1"]?.capacity.active).toBe(0);
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

  it("rejects stale REST capacity after a live language update", () => {
    const subject = store();
    subject.getState().mergeTaskLspLanguage({
      ...language(9, "ready"),
      capacity: { active: 1, queued: 0, limit: 4, revision: 2 },
    } as TaskLspLanguageSnapshot);
    subject.getState().setTaskLspSnapshot({
      task_id: "task-1",
      languages: [language(8, "starting")],
      capacity: { active: 0, queued: 0, limit: 4, revision: 1 } as TaskLspCapacity,
    });
    expect(subject.getState().taskLsp.byTaskId["task-1"]?.capacity.active).toBe(1);
  });
});

describe("LSP capacity sequencing", () => {
  it("orders capacity across backend restart epochs", () => {
    const subject = store();
    subject.getState().mergeTaskLspLanguage({
      ...language(9, "ready"),
      capacity: {
        active: 1,
        queued: 0,
        limit: 4,
        revision: 8,
        epoch: PRIOR_CAPACITY_EPOCH,
      },
    } as TaskLspLanguageSnapshot);
    subject.getState().setTaskLspSnapshot({
      task_id: "task-1",
      languages: [language(9, "ready")],
      capacity: {
        active: 0,
        queued: 0,
        limit: 4,
        revision: 1,
        epoch: CAPACITY_EPOCH,
      } as TaskLspCapacity,
    });
    subject.getState().mergeTaskLspLanguage({
      ...language(10, "ready"),
      capacity: {
        active: 2,
        queued: 0,
        limit: 4,
        revision: 99,
        epoch: PRIOR_CAPACITY_EPOCH,
      },
    } as TaskLspLanguageSnapshot);
    expect(subject.getState().taskLsp.byTaskId["task-1"]?.capacity.active).toBe(0);
  });

  it("accepts a lower-clock backend epoch only from an authoritative snapshot", () => {
    const subject = store();
    const previousEpoch = CAPACITY_EPOCH;
    const restartedEpoch = RESTARTED_CAPACITY_EPOCH;
    subject.getState().mergeTaskLspLanguage({
      ...language(9),
      capacity: { active: 1, queued: 0, limit: 4, revision: 8, epoch: previousEpoch },
    });

    subject.getState().setTaskLspSnapshot({
      task_id: "task-1",
      languages: [language(10)],
      capacity: { active: 0, queued: 0, limit: 4, revision: 1, epoch: restartedEpoch },
    });
    subject.getState().mergeTaskLspLanguage({
      ...language(11),
      capacity: { active: 2, queued: 0, limit: 4, revision: 99, epoch: previousEpoch },
    });

    expect(subject.getState().taskLsp.byTaskId["task-1"]?.capacity).toMatchObject({
      active: 0,
      epoch: restartedEpoch,
      revision: 1,
    });
  });

  it("rejects a retired epoch from a delayed authoritative snapshot", () => {
    const subject = store();
    const previousEpoch = CAPACITY_EPOCH;
    const restartedEpoch = RESTARTED_CAPACITY_EPOCH;
    subject.getState().mergeTaskLspLanguage({
      ...language(9),
      capacity: { active: 1, queued: 0, limit: 4, revision: 8, epoch: previousEpoch },
    });
    subject.getState().setTaskLspSnapshot({
      task_id: "task-1",
      languages: [language(10)],
      capacity: { active: 0, queued: 0, limit: 4, revision: 1, epoch: restartedEpoch },
    });

    subject.getState().setTaskLspSnapshot({
      task_id: "task-1",
      languages: [language(10)],
      capacity: { active: 3, queued: 0, limit: 4, revision: 99, epoch: previousEpoch },
    });

    expect(subject.getState().taskLsp.byTaskId["task-1"]?.capacity).toMatchObject({
      active: 0,
      epoch: restartedEpoch,
      revision: 1,
    });
  });

  it("accepts sequenced capacity after an unsequenced snapshot", () => {
    expectSequencedCapacityAfterUnsequencedSnapshot();
  });
});

describe("LSP live evidence", () => {
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

  it("clears control errors only when authoritative language evidence is accepted", () => {
    const subject = store();
    subject.getState().mergeTaskLspLanguage(language(9, "starting"));
    subject.getState().setTaskLspError("task-1", "start failed");

    subject.getState().mergeTaskLspLanguage(language(8, "ready"));
    expect(subject.getState().taskLsp.byTaskId["task-1"]?.error).toBe("start failed");

    subject.getState().mergeTaskLspLanguage(language(10, "ready"));
    expect(subject.getState().taskLsp.byTaskId["task-1"]?.error).toBeNull();
  });

  it("does not clear a newer error with a snapshot from an older error epoch", () => {
    const subject = store();
    subject.getState().setTaskLspSnapshot(
      {
        task_id: "task-1",
        languages: [language(1)],
        capacity: { active: 1, queued: 0, limit: 4 },
      },
      0,
    );
    subject.getState().setTaskLspError("task-1", STOP_FAILED);

    subject.getState().setTaskLspSnapshot(
      {
        task_id: "task-1",
        languages: [language(1)],
        capacity: { active: 1, queued: 0, limit: 4 },
      },
      0,
    );
    expect(subject.getState().taskLsp.byTaskId["task-1"]?.error).toBe(STOP_FAILED);

    subject.getState().setTaskLspSnapshot(
      {
        task_id: "task-1",
        languages: [language(1)],
        capacity: { active: 1, queued: 0, limit: 4 },
      },
      1,
    );
    expect(subject.getState().taskLsp.byTaskId["task-1"]?.error).toBeNull();
  });

  it("clears a load failure without advancing the control error epoch", () => {
    const subject = store();
    subject.getState().setTaskLspLoadError("task-1", "refresh failed", 0);

    expect(subject.getState().taskLsp.byTaskId["task-1"]).toMatchObject({
      error: "refresh failed",
      controlErrorEpoch: 0,
    });

    subject.getState().setTaskLspSnapshot(
      {
        task_id: "task-1",
        languages: [language(1)],
        capacity: { active: 1, queued: 0, limit: 4 },
      },
      0,
    );
    expect(subject.getState().taskLsp.byTaskId["task-1"]?.error).toBeNull();
  });

  it("does not replace a newer control failure with an older load failure", () => {
    const subject = store();
    subject.getState().setTaskLspError("task-1", STOP_FAILED);
    subject.getState().setTaskLspLoadError("task-1", "refresh failed", 0);

    expect(subject.getState().taskLsp.byTaskId["task-1"]?.error).toBe(STOP_FAILED);
  });
});
