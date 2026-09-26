import { describe, expect, it } from "vitest";

import type { RunStatus } from "@/lib/types/automation";
import {
  ALL_STATUSES,
  groupRunsByState,
  isOpenRun,
  RUN_STATUS_LABEL_KEY,
  STATUS_FILTER_OPTIONS,
  statusDotClass,
  statusLabelKey,
} from "./run-status";
import { ANY_AUTOMATION, isDefaultFilters } from "./run-filters";

const EVERY_STATUS: RunStatus[] = [
  "triggered",
  "task_created",
  "succeeded",
  "failed",
  "skipped",
  "archived",
  "cancelled",
];

describe("run status presentation", () => {
  it("labels and colours every status the backend can derive", () => {
    // The status set includes two read-time-derived values (archived,
    // cancelled). Missing either one renders a blank label on a real run.
    for (const status of EVERY_STATUS) {
      expect(statusLabelKey(status), status).toBeTruthy();
      expect(statusDotClass(status), status).toBeTruthy();
    }
  });

  it("calls a task_created run Running, not by its storage name", () => {
    expect(statusLabelKey("task_created")).toBe("automations:runRunning");
  });

  it("falls back rather than rendering blank for an unknown status", () => {
    const unknown = "not-a-status" as RunStatus;
    expect(statusLabelKey(unknown)).toBe(RUN_STATUS_LABEL_KEY.triggered);
    expect(statusDotClass(unknown)).toBeTruthy();
  });
});

describe("status filter options", () => {
  it("offers Skipped, which is otherwise invisible", () => {
    // A scheduled firing turned away by the concurrency cap writes a run row
    // and nothing else, so without this filter a jammed automation looks
    // identical to one that was never due.
    expect(STATUS_FILTER_OPTIONS.map((o) => o.value)).toContain("skipped");
  });

  it("offers the derived terminal statuses too", () => {
    const values = STATUS_FILTER_OPTIONS.map((o) => o.value);
    expect(values).toContain("archived");
    expect(values).toContain("cancelled");
  });

  it("offers every status the feed can display", () => {
    // A status that renders but cannot be filtered leaves the reader unable to
    // narrow to it, so the option list tracks the label map rather than a
    // hand-picked subset.
    const values = STATUS_FILTER_OPTIONS.map((o) => o.value);
    for (const status of EVERY_STATUS) {
      expect(values, status).toContain(status);
    }
  });

  it("leads with All and never repeats a value", () => {
    expect(STATUS_FILTER_OPTIONS[0].value).toBe(ALL_STATUSES);
    const values = STATUS_FILTER_OPTIONS.map((o) => o.value);
    expect(new Set(values).size).toBe(values.length);
  });

  it("labels each option the same way the feed labels the run", () => {
    // Compared against the literal rather than ALL_STATUSES: that const is
    // widened to RunStatusFilter, so it does not narrow the union here.
    for (const option of STATUS_FILTER_OPTIONS) {
      if (option.value === "all") continue;
      expect(option.labelKey).toBe(RUN_STATUS_LABEL_KEY[option.value]);
    }
  });
});

describe("isOpenRun", () => {
  it("counts only the two statuses that mean the run has not finished", () => {
    expect(isOpenRun("triggered")).toBe(true);
    expect(isOpenRun("task_created")).toBe(true);
    for (const status of EVERY_STATUS) {
      if (status === "triggered" || status === "task_created") continue;
      expect(isOpenRun(status), status).toBe(false);
    }
  });

  it("treats the read-time-derived terminals as finished", () => {
    // A cancelled or archived run is over. Counting either as open would hold
    // an automation at its concurrency cap forever.
    expect(isOpenRun("archived")).toBe(false);
    expect(isOpenRun("cancelled")).toBe(false);
  });
});

describe("groupRunsByState", () => {
  const RUNS = [
    { id: "a", status: "succeeded" as RunStatus },
    { id: "b", status: "task_created" as RunStatus },
    { id: "c", status: "failed" as RunStatus },
    { id: "d", status: "triggered" as RunStatus },
  ];

  it("separates what is happening from what already happened", () => {
    const { running, completed } = groupRunsByState(RUNS);

    expect(running.map((run) => run.id)).toEqual(["b", "d"]);
    expect(completed.map((run) => run.id)).toEqual(["a", "c"]);
  });

  it("keeps every run — this groups, it does not filter", () => {
    const { running, completed } = groupRunsByState(RUNS);

    expect(running.length + completed.length).toBe(RUNS.length);
  });

  it("preserves the order it was given within each group", () => {
    const { completed } = groupRunsByState(RUNS);

    expect(completed.map((run) => run.id)).toEqual(["a", "c"]);
  });

  it("handles an empty history", () => {
    expect(groupRunsByState([])).toEqual({ running: [], completed: [] });
  });
});

describe("isDefaultFilters", () => {
  it("is true only when neither filter narrows anything", () => {
    // The "N of M" counter and the clear-filters affordance both hang off this,
    // so a wrong answer either hides the fact that a filter is on or shows the
    // counter permanently.
    expect(isDefaultFilters(ALL_STATUSES, ANY_AUTOMATION)).toBe(true);
    expect(isDefaultFilters("failed", ANY_AUTOMATION)).toBe(false);
    expect(isDefaultFilters(ALL_STATUSES, "automation-1")).toBe(false);
    expect(isDefaultFilters("failed", "automation-1")).toBe(false);
  });
});
