import { expect, it } from "vitest";
import { activateLocale } from "@/lib/i18n";
import type { LspProgressSnapshot } from "./lsp-progress";
import {
  formatLspElapsed,
  getLspCompactSummary,
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
  expect(formatLspElapsed(-1)).toBe("0 sec");
  expect(formatLspElapsed(9_900)).toBe("9 sec");
  expect(formatLspElapsed(65_000)).toBe("1 min 05 sec");
  expect(formatLspElapsed(3_725_000)).toBe("1 hr 02 min");
});

it("localizes elapsed units and their composition", async () => {
  try {
    await activateLocale("pseudo");
    expect(formatLspElapsed(65_000)).toBe("1 ḿĩń 05 śēć");
  } finally {
    await activateLocale("en");
  }
});

it("formats compact live summaries for the application status bar", () => {
  expect(
    getLspCompactSummary(
      { state: "starting" },
      { ...EMPTY_PROGRESS, initializingSince: 2_000 },
      67_000,
    ),
  ).toBe("Server process started · 1 min 05 sec");

  expect(
    getLspCompactSummary(
      { state: "ready" },
      {
        ...EMPTY_PROGRESS,
        active: [
          {
            token: "import",
            title: "Importing project",
            message: "Resolving modules",
            percentage: 42,
            startedAt: 2_000,
          },
        ],
      },
      67_000,
    ),
  ).toBe("Importing project · 42%");

  expect(getLspCompactSummary({ state: "ready" }, EMPTY_PROGRESS, 67_000)).toBe("Connected");
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
    elapsed: "1 min 05 sec",
    concurrentCount: 2,
  });
});

it("discloses that the server process launched while initialize is pending", () => {
  const progress = { ...EMPTY_PROGRESS, initializingSince: 2_000 };

  expect(getLspConnectionLabel({ state: "starting" }, progress)).toBe("Server process started");
  expect(getLspProgressView({ state: "starting" }, progress, 61_999, "kotlin")).toEqual({
    kind: "initializing",
    stage: "initialize_pending",
    title: "Server process started",
    description: "Waiting for the language server to respond to the LSP initialize request.",
    guidance: "Cross-file features become available after initialization completes.",
    elapsed: "59 sec",
  });
  expect(getLspLifecycleAction({ state: "starting" })).toEqual({
    label: "Stop",
    enabled: true,
  });
});

it("warns at 60 seconds without timing out or inventing Kotlin progress", () => {
  const view = getLspProgressView(
    { state: "starting" },
    { ...EMPTY_PROGRESS, initializingSince: 2_000 },
    62_000,
    "kotlin",
  );

  expect(view).toEqual({
    kind: "initializing",
    stage: "long_running",
    title: "Initialization is taking longer than usual",
    description: "The server process is still running while Kandev waits for LSP initialize.",
    guidance:
      "Kotlin LSP may be importing the Gradle project. Cross-file features remain unavailable until initialization completes.",
    elapsed: "1 min 00 sec",
  });
  expect(JSON.stringify(view).toLowerCase()).not.toMatch(/eta|time remaining|percentage|indexing/);
  expect(getLspLifecycleAction({ state: "starting" })).toEqual({
    label: "Stop",
    enabled: true,
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
