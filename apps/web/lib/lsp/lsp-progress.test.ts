import { expect, it } from "vitest";
import {
  applyLspProgress,
  createLspProgressSnapshot,
  finishLspInitialization,
  isLspProgressToken,
} from "./lsp-progress";

it("starts initialization and clears only its local pending state", () => {
  const snapshot = createLspProgressSnapshot(100);
  const active = applyLspProgress(
    snapshot,
    new Set(["initialize"]),
    {
      token: "initialize",
      value: { kind: "begin", title: "Importing project", percentage: 15 },
    },
    120,
  );

  expect(active.initializingSince).toBe(100);
  expect(finishLspInitialization(active)).toEqual({
    ...active,
    initializingSince: null,
  });
});

it("records begin work and clamps the server percentage", () => {
  const snapshot = createLspProgressSnapshot(100);
  const next = applyLspProgress(
    snapshot,
    new Set(["initialize"]),
    {
      token: "initialize",
      value: {
        kind: "begin",
        title: "Scanning Kotlin",
        message: "Resolving modules",
        percentage: 125,
      },
    },
    150,
  );

  expect(next).toEqual({
    initializingSince: 100,
    active: [
      {
        token: "initialize",
        title: "Scanning Kotlin",
        message: "Resolving modules",
        percentage: 100,
        startedAt: 150,
      },
    ],
    completed: null,
    hasReportedProgress: true,
  });
  expect(snapshot.active).toEqual([]);
});

it("preserves omitted report fields and clamps a supplied percentage", () => {
  const registered = new Set<string | number>(["initialize"]);
  const begun = applyLspProgress(
    createLspProgressSnapshot(100),
    registered,
    {
      token: "initialize",
      value: {
        kind: "begin",
        title: "Indexing",
        message: "Reading sources",
        percentage: 40,
      },
    },
    120,
  );
  const withoutFields = applyLspProgress(
    begun,
    registered,
    { token: "initialize", value: { kind: "report" } },
    130,
  );
  const updated = applyLspProgress(
    withoutFields,
    registered,
    {
      token: "initialize",
      value: { kind: "report", message: "Resolving symbols", percentage: -5 },
    },
    140,
  );

  expect(withoutFields.active[0]).toEqual(begun.active[0]);
  expect(updated.active[0]).toEqual({
    ...begun.active[0],
    message: "Resolving symbols",
    percentage: 0,
  });
});

it("keeps concurrent string and numeric tokens independent and ordered by begin", () => {
  const registered = new Set<string | number>(["initialize", 7]);
  const first = applyLspProgress(
    createLspProgressSnapshot(100),
    registered,
    {
      token: "initialize",
      value: { kind: "begin", title: "Project import" },
    },
    110,
  );
  const second = applyLspProgress(
    first,
    registered,
    {
      token: 7,
      value: { kind: "begin", title: "Dependency analysis", percentage: 10 },
    },
    120,
  );
  const reported = applyLspProgress(
    second,
    registered,
    { token: 7, value: { kind: "report", percentage: 60 } },
    130,
  );
  const firstEnded = applyLspProgress(
    reported,
    registered,
    {
      token: "initialize",
      value: { kind: "end", message: "Project model loaded" },
    },
    140,
  );
  const allEnded = applyLspProgress(
    firstEnded,
    registered,
    { token: 7, value: { kind: "end" } },
    150,
  );

  expect(reported.active.map(({ token, percentage }) => ({ token, percentage }))).toEqual([
    { token: "initialize", percentage: null },
    { token: 7, percentage: 60 },
  ]);
  expect(firstEnded.active.map((item) => item.token)).toEqual([7]);
  expect(firstEnded.completed).toEqual({
    token: "initialize",
    title: "Project import",
    message: "Project model loaded",
    startedAt: 110,
    completedAt: 140,
  });
  expect(allEnded.active).toEqual([]);
  expect(allEnded.completed).toEqual({
    token: 7,
    title: "Dependency analysis",
    message: null,
    startedAt: 120,
    completedAt: 150,
  });
});

it("ignores unknown tokens and malformed payloads without changing snapshot identity", () => {
  const snapshot = createLspProgressSnapshot(100);
  const registered = new Set<string | number>(["known"]);
  const malformed = [
    null,
    {},
    { token: "unknown", value: { kind: "begin", title: "Ignored" } },
    { token: {}, value: { kind: "begin", title: "Ignored" } },
    { token: "known", value: null },
    { token: "known", value: { kind: "begin" } },
    { token: "known", value: { kind: "begin", title: "Work", message: 1 } },
    { token: "known", value: { kind: "begin", title: "Work", percentage: Number.NaN } },
    { token: "known", value: { kind: "report", percentage: "50" } },
    { token: "known", value: { kind: "end", message: false } },
  ];

  for (const params of malformed) {
    expect(applyLspProgress(snapshot, registered, params, 200)).toBe(snapshot);
  }
  expect(isLspProgressToken("token")).toBe(true);
  expect(isLspProgressToken(1)).toBe(true);
  expect(isLspProgressToken(Number.POSITIVE_INFINITY)).toBe(false);
  expect(isLspProgressToken({})).toBe(false);
});
