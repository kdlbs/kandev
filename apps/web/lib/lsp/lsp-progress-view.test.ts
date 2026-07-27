import { expect, it } from "vitest";
import type { LspProgressSnapshot } from "./lsp-progress";
import {
  formatLspElapsed,
  getLspConnectionLabel,
  getLspLifecycleAction,
  getLspProgressView,
} from "./lsp-progress-view";

const EMPTY_PROGRESS: LspProgressSnapshot = {
  initializingSince: null,
  active: [],
  completed: null,
  hasReportedProgress: false,
};

it("formats elapsed time without estimating time remaining", () => {
  expect(formatLspElapsed(-1)).toBe("0s");
  expect(formatLspElapsed(9_900)).toBe("9s");
  expect(formatLspElapsed(65_000)).toBe("1m 05s");
  expect(formatLspElapsed(3_725_000)).toBe("1h 02m");
});

it("keeps connection labels and lifecycle actions separate from project work", () => {
  expect(getLspConnectionLabel({ state: "ready" })).toBe("Connected");
  expect(getLspConnectionLabel({ state: "installing" })).toBe("Installing language server");
  expect(getLspConnectionLabel({ state: "error", reason: "crashed" })).toBe("Error");

  expect(getLspLifecycleAction({ state: "disabled" })).toEqual({
    label: "Start",
    enabled: true,
  });
  expect(getLspLifecycleAction({ state: "ready" })).toEqual({
    label: "Stop",
    enabled: true,
  });
  expect(getLspLifecycleAction({ state: "error", reason: "crashed" })).toEqual({
    label: "Retry",
    enabled: true,
  });
  expect(getLspLifecycleAction({ state: "stopping" })).toEqual({
    label: "Stopping",
    enabled: false,
  });
});

it("presents the oldest active server item without averaging concurrent work", () => {
  const progress: LspProgressSnapshot = {
    initializingSince: 1_000,
    active: [
      {
        token: "first",
        title: "Importing project",
        message: "Resolving modules",
        percentage: 42,
        startedAt: 2_000,
      },
      {
        token: "second",
        title: "Scanning dependencies",
        message: null,
        percentage: 90,
        startedAt: 3_000,
      },
    ],
    completed: null,
    hasReportedProgress: true,
  };

  expect(getLspProgressView({ state: "starting" }, progress, 67_000)).toEqual({
    kind: "active",
    title: "Importing project",
    message: "Resolving modules",
    percentage: 42,
    elapsed: "1m 05s",
    concurrentCount: 2,
  });
});

it("shows locally timed initialization without inventing server progress", () => {
  expect(
    getLspProgressView(
      { state: "starting" },
      { ...EMPTY_PROGRESS, initializingSince: 2_000 },
      11_000,
    ),
  ).toEqual({
    kind: "initializing",
    title: "Preparing project…",
    description: "Waiting for the language server to finish initializing.",
    elapsed: "9s",
  });
});

it("states when a ready server has not reported background analysis", () => {
  expect(getLspProgressView({ state: "ready" }, EMPTY_PROGRESS, 10_000)).toEqual({
    kind: "idle",
    title: "No background work reported",
    description:
      "The language server has not reported ongoing project analysis. Cross-file results may still warm up.",
  });
});

it("describes completion as server-reported work rather than full indexing", () => {
  const progress: LspProgressSnapshot = {
    ...EMPTY_PROGRESS,
    hasReportedProgress: true,
    completed: {
      token: 1,
      title: "Project import",
      message: "Import finished",
      startedAt: 1_000,
      completedAt: 3_000,
    },
  };

  expect(getLspProgressView({ state: "ready" }, progress, 4_000)).toEqual({
    kind: "completed",
    title: "Server-reported work finished",
    workTitle: "Project import",
    message: "Import finished",
  });
});
