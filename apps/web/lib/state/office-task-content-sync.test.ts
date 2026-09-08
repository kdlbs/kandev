import { beforeEach, describe, expect, it } from "vitest";
import {
  __resetOfficeTaskContentSyncForTests,
  beginWrite,
  closeFieldEditor,
  endAllEditorsForTask,
  endWrite,
  getCanonicalValue,
  isFieldGuarded,
  nextTaskSequence,
  openFieldEditor,
  parseUpdatedAt,
  recordRefetchCandidate,
  recordWriteSuccess,
  seedInitialCanonical,
  shouldRestoreAfterFailedWrite,
  subscribeField,
} from "./office-task-content-sync";

const TASK = "task-1";
const T10 = "2026-08-20T10:00:00Z";
const T11 = "2026-08-20T11:00:00Z";
const T12 = "2026-08-20T12:00:00Z";

beforeEach(() => {
  __resetOfficeTaskContentSyncForTests();
});

describe("parseUpdatedAt", () => {
  it("parses the SQLite space-separated offset-bearing layout to nanosecond precision (AC-66)", () => {
    const ns = parseUpdatedAt("2026-08-20 14:30:05.123456789+00:00");
    const wholeSecondMs = Date.parse("2026-08-20T14:30:05+00:00");
    expect(ns).toBe(BigInt(wholeSecondMs) * BigInt(1_000_000) + BigInt(123456789));
  });

  it("parses the RFC3339Nano T/Z layout emitted by a write response to nanosecond precision (AC-66)", () => {
    const ns = parseUpdatedAt("2026-08-20T14:30:05.123456789Z");
    const wholeSecondMs = Date.parse("2026-08-20T14:30:05Z");
    expect(ns).toBe(BigInt(wholeSecondMs) * BigInt(1_000_000) + BigInt(123456789));
  });

  it("treats the space-separated form's offset as authoritative, matching its T/Z equivalent", () => {
    const spaceForm = parseUpdatedAt("2026-08-20 14:30:05.123456789+00:00");
    const tForm = parseUpdatedAt("2026-08-20T14:30:05.123456789Z");
    expect(spaceForm).toBe(tForm);
  });

  it("resolves the space-separated form via its own embedded offset, not the host machine's local timezone (AC-66)", () => {
    // Every other space-separated fixture above uses a +00:00 offset, which
    // coincidentally equals local time on a UTC-TZ test runner and would not
    // catch a regression to `new Date(rawSpaceSeparatedString)` — the spec
    // explicitly calls out that browsers disagree on how (or whether) to
    // parse that non-ISO-8601 form, with some falling back to local time.
    // A non-zero offset makes the correct answer distinguishable from any
    // such fallback regardless of what timezone the test happens to run in.
    const ns = parseUpdatedAt("2026-08-20 14:30:05.123456789+05:30");
    // 14:30:05+05:30 is 09:00:05Z — computed via an explicit offset, so this
    // expected value is itself timezone-independent.
    const wholeSecondMs = Date.parse("2026-08-20T09:00:05+00:00");
    expect(ns).toBe(BigInt(wholeSecondMs) * BigInt(1_000_000) + BigInt(123456789));
  });

  it("does not collapse two distinct sub-millisecond instants to the same value", () => {
    // AC-60/62 requires strict ordering between these two: both round to the
    // same millisecond under Date.parse (123ms), but differ at the
    // microsecond digit — a millisecond-truncating parse would wrongly
    // report them equal.
    const earlier = parseUpdatedAt("2026-08-20T14:30:05.123400000Z");
    const later = parseUpdatedAt("2026-08-20T14:30:05.123500000Z");
    expect(earlier).not.toBe(later);
    expect(earlier! < later!).toBe(true);
  });

  it("returns null for an unparseable value (AC-67 candidate)", () => {
    expect(parseUpdatedAt("not-a-date")).toBeNull();
    expect(parseUpdatedAt("")).toBeNull();
    expect(parseUpdatedAt(null)).toBeNull();
    expect(parseUpdatedAt(undefined)).toBeNull();
  });
});

describe("seedInitialCanonical (AC-59, AC-72)", () => {
  it("records the first candidate unconditionally, even if its updated_at is unparseable", () => {
    seedInitialCanonical(TASK, "title", "Hello", "garbage");
    expect(getCanonicalValue(TASK, "title")).toBe("Hello");
  });

  it("a same-session revisit re-seed overwrites a stale canonical with a newer value (AC-59, AC-64)", () => {
    // A revisit's fresh initial-load seed must be able to supersede a
    // canonical value recorded by an earlier visit in the same SPA session —
    // otherwise a later failed commit's restore (AC-9/AC-64, which reads the
    // canonical value live at restore time) could resurrect a value the
    // server has already moved past instead of the value most recently
    // loaded from it.
    seedInitialCanonical(TASK, "title", "Hello", T10);
    seedInitialCanonical(TASK, "title", "Newer on revisit", T11);
    expect(getCanonicalValue(TASK, "title")).toBe("Newer on revisit");
  });

  it("a revisit re-seed does not resurrect a stale value once a newer one is canonical", () => {
    // The revisit seed goes through the same AC-60/62 ordering as any other
    // candidate — it is not a blind overwrite, just no longer a no-op.
    seedInitialCanonical(TASK, "title", "Hello", T11);
    seedInitialCanonical(TASK, "title", "Stale revisit", T10);
    expect(getCanonicalValue(TASK, "title")).toBe("Hello");
  });

  it("normalises an absent description to the empty string (AC-71)", () => {
    seedInitialCanonical(TASK, "description", undefined as unknown as string, T10);
    expect(getCanonicalValue(TASK, "description")).toBe("");
  });

  it("records the first candidate unconditionally even when its sequence predates the field's last write (AC-72 overrides AC-52)", () => {
    // AC-72: until a field has a recorded canonical value, the seed rule
    // exempts the next candidate from every other rule, including the AC-52
    // staleness check — that check compares against a write, but there is
    // nothing yet to be stale relative to. This reproduces the initial page
    // load's seed call landing after a title commit was already issued
    // (e.g. the authoritative GET resolves late, or is retried, while the
    // page already rendered the task interactively from a store cache).
    const staleSeq = nextTaskSequence(TASK);
    const writeSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", writeSeq);

    const accepted = recordRefetchCandidate(TASK, "title", "first ever value", T10, staleSeq);

    expect(accepted).toBe(true);
    expect(getCanonicalValue(TASK, "title")).toBe("first ever value");
  });
});

describe("recordRefetchCandidate ordering (AC-60, AC-62, AC-63, AC-73)", () => {
  it("discards a strictly older candidate (AC-60)", () => {
    seedInitialCanonical(TASK, "title", "v1", T10);
    const accepted = recordRefetchCandidate(
      TASK,
      "title",
      "older",
      "2026-08-20T09:00:00Z",
      nextTaskSequence(TASK),
    );
    expect(accepted).toBe(false);
    expect(getCanonicalValue(TASK, "title")).toBe("v1");
  });

  it("accepts a strictly newer candidate", () => {
    seedInitialCanonical(TASK, "title", "v1", T10);
    const accepted = recordRefetchCandidate(TASK, "title", "newer", T11, nextTaskSequence(TASK));
    expect(accepted).toBe(true);
    expect(getCanonicalValue(TASK, "title")).toBe("newer");
  });

  it("breaks an equal-instant tie by request-issue sequence, higher sequence wins (AC-63)", () => {
    seedInitialCanonical(TASK, "title", "v1", T10);
    const loserSeq = nextTaskSequence(TASK);
    const winnerSeq = nextTaskSequence(TASK);
    // Winner recorded first, loser arrives after but carries a lower sequence.
    expect(recordRefetchCandidate(TASK, "title", "winner", T10, winnerSeq)).toBe(true);
    expect(getCanonicalValue(TASK, "title")).toBe("winner");
    expect(recordRefetchCandidate(TASK, "title", "loser", T10, loserSeq)).toBe(false);
    expect(getCanonicalValue(TASK, "title")).toBe("winner");
  });

  it("discards a candidate whose updated_at cannot be parsed (AC-67)", () => {
    seedInitialCanonical(TASK, "title", "v1", T10);
    const accepted = recordRefetchCandidate(
      TASK,
      "title",
      "bad",
      "garbage",
      nextTaskSequence(TASK),
    );
    expect(accepted).toBe(false);
    expect(getCanonicalValue(TASK, "title")).toBe("v1");
  });

  it("an unparseable seed loses to any later candidate that does parse (AC-73)", () => {
    seedInitialCanonical(TASK, "title", "seed", "garbage");
    const accepted = recordRefetchCandidate(
      TASK,
      "title",
      "supersedes",
      T10,
      nextTaskSequence(TASK),
    );
    expect(accepted).toBe(true);
    expect(getCanonicalValue(TASK, "title")).toBe("supersedes");
  });

  it("discards a stale SQLite-layout refetch that lands within the same millisecond as an already-recorded RFC3339Nano write (AC-60/62/66)", () => {
    // Both instants round to the same millisecond, so a millisecond-precision
    // comparison would wrongly treat them as an equal-instant tie (or worse,
    // accept the stale one outright) instead of correctly ordering them.
    seedInitialCanonical(TASK, "title", "seed", T10);
    const writeSeq = nextTaskSequence(TASK);
    expect(
      recordWriteSuccess(TASK, "title", "from write", "2026-08-20T14:30:05.123500000Z", writeSeq),
    ).toBe(true);
    endWrite(TASK, "title", writeSeq);

    const refetchSeq = nextTaskSequence(TASK);
    const accepted = recordRefetchCandidate(
      TASK,
      "title",
      "from stale refetch",
      "2026-08-20 14:30:05.123400000+00:00",
      refetchSeq,
    );
    expect(accepted).toBe(false);
    expect(getCanonicalValue(TASK, "title")).toBe("from write");
  });

  it("accepts an SQLite-layout refetch that truly postdates an RFC3339Nano write by sub-millisecond microseconds (AC-60/62/66)", () => {
    seedInitialCanonical(TASK, "title", "seed", T10);
    const writeSeq = nextTaskSequence(TASK);
    expect(
      recordWriteSuccess(TASK, "title", "from write", "2026-08-20T14:30:05.123400000Z", writeSeq),
    ).toBe(true);
    endWrite(TASK, "title", writeSeq);

    const refetchSeq = nextTaskSequence(TASK);
    const accepted = recordRefetchCandidate(
      TASK,
      "title",
      "from later refetch",
      "2026-08-20 14:30:05.123500000+00:00",
      refetchSeq,
    );
    expect(accepted).toBe(true);
    expect(getCanonicalValue(TASK, "title")).toBe("from later refetch");
  });
});

describe("recordRefetchCandidate staleness vs a committed write (AC-52)", () => {
  it("discards a refetch whose request predates this client's most recent write to the field", () => {
    seedInitialCanonical(TASK, "title", "v1", T10);
    const refetchSeq = nextTaskSequence(TASK);
    const writeSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", writeSeq);

    const accepted = recordRefetchCandidate(TASK, "title", "stale", T12, refetchSeq);
    expect(accepted).toBe(false);
    expect(getCanonicalValue(TASK, "title")).toBe("v1");
  });

  it("does not apply write-staleness to a write's own response (AC-52 scoped to refetch only)", () => {
    seedInitialCanonical(TASK, "title", "v1", T10);
    const writeSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", writeSeq);
    // A later write is also issued for the same field, making this write's
    // sequence no longer "most recent" — its own response must still record.
    const laterWriteSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", laterWriteSeq);

    const accepted = recordWriteSuccess(TASK, "title", "first-write-result", T11, writeSeq);
    expect(accepted).toBe(true);
  });

  it("still records a non-stale refetch while a write to that field is in flight (AC-49)", () => {
    seedInitialCanonical(TASK, "title", "v1", T10);
    const writeSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", writeSeq);
    const refetchSeq = nextTaskSequence(TASK);

    const accepted = recordRefetchCandidate(TASK, "title", "v2-from-refetch", T11, refetchSeq);

    expect(accepted).toBe(true);
    expect(getCanonicalValue(TASK, "title")).toBe("v2-from-refetch");
    // The write is still in flight, so the field remains guarded: recording
    // succeeded, but nothing was applied to the displayed value yet (AC-49
    // scoped to recording; AC-50 governs the deferred apply on release).
    expect(isFieldGuarded(TASK, "title")).toBe(true);
  });
});

describe("guard derivation (AC-28, AC-38, AC-68, AC-69, AC-70)", () => {
  it("is unguarded by default", () => {
    expect(isFieldGuarded(TASK, "title")).toBe(false);
  });

  it("is guarded while an editor is open", () => {
    openFieldEditor(TASK, "title");
    expect(isFieldGuarded(TASK, "title")).toBe(true);
    closeFieldEditor(TASK, "title");
    expect(isFieldGuarded(TASK, "title")).toBe(false);
  });

  it("is guarded while a write is in flight, independent of editor state", () => {
    const seq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", seq);
    expect(isFieldGuarded(TASK, "title")).toBe(true);
    endWrite(TASK, "title", seq);
    expect(isFieldGuarded(TASK, "title")).toBe(false);
  });

  it("stays guarded until both the editor closes and the write resolves", () => {
    openFieldEditor(TASK, "title");
    const seq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", seq);
    closeFieldEditor(TASK, "title");
    expect(isFieldGuarded(TASK, "title")).toBe(true);
    endWrite(TASK, "title", seq);
    expect(isFieldGuarded(TASK, "title")).toBe(false);
  });

  it("endAllEditorsForTask ends only the open-editor contribution, not pending writes (AC-68)", () => {
    openFieldEditor(TASK, "title");
    const seq = nextTaskSequence(TASK);
    beginWrite(TASK, "description", seq);
    openFieldEditor(TASK, "description");

    endAllEditorsForTask(TASK);

    expect(isFieldGuarded(TASK, "title")).toBe(false);
    expect(isFieldGuarded(TASK, "description")).toBe(true); // write still pending
    endWrite(TASK, "description", seq);
    expect(isFieldGuarded(TASK, "description")).toBe(false);
  });

  it("a write resolving after unmount still records canonical and clears its guard (AC-69)", () => {
    const seq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", seq);
    endAllEditorsForTask(TASK); // simulate unmount; write is still in flight

    const accepted = recordWriteSuccess(TASK, "title", "post-unmount", T12, seq);
    expect(accepted).toBe(true);
    endWrite(TASK, "title", seq);
    expect(isFieldGuarded(TASK, "title")).toBe(false);
    expect(getCanonicalValue(TASK, "title")).toBe("post-unmount");
  });

  it("ends a write's guard contribution the same way on failure as on success (AC-70)", () => {
    const seq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", seq);
    // Failure path: caller does NOT call recordWriteSuccess, only endWrite.
    endWrite(TASK, "title", seq);
    expect(isFieldGuarded(TASK, "title")).toBe(false);
  });
});

describe("shouldRestoreAfterFailedWrite (AC-9, AC-44, AC-53, R5-F6)", () => {
  it("restores when no other commit to the field is pending or already succeeded", () => {
    const seq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", seq);
    expect(shouldRestoreAfterFailedWrite(TASK, "title", seq)).toBe(true);
  });

  it("suppresses restore when a later-sequenced commit already succeeded (AC-44)", () => {
    const failingSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", failingSeq);
    const laterSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", laterSeq);
    recordWriteSuccess(TASK, "title", "later-wins", T11, laterSeq);
    endWrite(TASK, "title", laterSeq);

    expect(shouldRestoreAfterFailedWrite(TASK, "title", failingSeq)).toBe(false);
  });

  it("suppresses restore when a later-sequenced commit is still unresolved (AC-53)", () => {
    const failingSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", failingSeq);
    const laterSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", laterSeq);

    expect(shouldRestoreAfterFailedWrite(TASK, "title", failingSeq)).toBe(false);
  });

  it("does not suppress restore because of an earlier-sequenced commit, resolved or not (R5-F6)", () => {
    const earlierSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", earlierSeq); // still unresolved
    const failingSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", failingSeq);

    expect(shouldRestoreAfterFailedWrite(TASK, "title", failingSeq)).toBe(true);
  });

  it("does not suppress restore when a later-sequenced commit also resolves with failure (AC-65)", () => {
    const failingSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", failingSeq);
    const laterSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", laterSeq);
    // The later commit also fails: caller does not call recordWriteSuccess, only endWrite.
    endWrite(TASK, "title", laterSeq);

    expect(shouldRestoreAfterFailedWrite(TASK, "title", failingSeq)).toBe(true);
  });

  it("chained double failure: earlier commit resolves first, restore is suppressed then completed by the later failure (AC-65)", () => {
    // This module never mutates canonical on failure — recordWriteSuccess is
    // the only writer, and it is deliberately never called here (both
    // commits fail). The two shouldRestoreAfterFailedWrite return values
    // below are the actual assertions; the real "does the displayed title
    // get restored to canonical" behavior is exercised end-to-end (via
    // useCommitTaskTitle's actual restore action) in
    // use-optimistic-task-mutation.test.tsx's "chained double failure" tests.
    seedInitialCanonical(TASK, "title", "original", T10);
    const earlySeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", earlySeq);
    const laterSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", laterSeq);

    expect(shouldRestoreAfterFailedWrite(TASK, "title", earlySeq)).toBe(false); // AC-53: later still pending
    endWrite(TASK, "title", earlySeq);

    expect(shouldRestoreAfterFailedWrite(TASK, "title", laterSeq)).toBe(true); // nothing left pending
    endWrite(TASK, "title", laterSeq);
  });

  it("chained double failure: later commit resolves first, both restores fire, canonical unaffected (AC-65)", () => {
    // See the comment on the previous test: this only exercises the boolean
    // query, not an actual restore. The end-to-end restore-to-canonical
    // behavior lives in use-optimistic-task-mutation.test.tsx.
    seedInitialCanonical(TASK, "title", "original", T10);
    const earlySeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", earlySeq);
    const laterSeq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", laterSeq);

    expect(shouldRestoreAfterFailedWrite(TASK, "title", laterSeq)).toBe(true); // nothing later pending
    endWrite(TASK, "title", laterSeq);

    expect(shouldRestoreAfterFailedWrite(TASK, "title", earlySeq)).toBe(true); // the later write already resolved
    endWrite(TASK, "title", earlySeq);
  });
});

describe("recordWriteSuccess ordering relative to guard release (AC-58)", () => {
  it("records the canonical value while the write's guard contribution is still held, before the caller releases it", () => {
    seedInitialCanonical(TASK, "title", "v1", T10);
    const seq = nextTaskSequence(TASK);
    beginWrite(TASK, "title", seq);

    recordWriteSuccess(TASK, "title", "v2", T11, seq);

    // Canonical must already reflect the persisted value even though the
    // write's guard contribution has not been released yet (endWrite has not
    // run) — AC-58 requires recording strictly before the guard releases,
    // not merely "eventually" once it does.
    expect(getCanonicalValue(TASK, "title")).toBe("v2");
    expect(isFieldGuarded(TASK, "title")).toBe(true);

    endWrite(TASK, "title", seq);
    expect(isFieldGuarded(TASK, "title")).toBe(false);
  });
});

describe("convergence after overlapping successful title commits (AC-54)", () => {
  it("converges on the later-persisted title when the earlier commit's response is recorded second", () => {
    const seen: string[] = [];
    seedInitialCanonical(TASK, "title", "original", T10);
    subscribeField(TASK, "title", (value) => seen.push(value));

    const seqA = nextTaskSequence(TASK);
    beginWrite(TASK, "title", seqA);
    const seqB = nextTaskSequence(TASK);
    beginWrite(TASK, "title", seqB);

    // B was issued after A, but its server response is recorded first while
    // A is still in flight, so the field stays guarded and nothing notifies yet.
    recordWriteSuccess(TASK, "title", "B-title", T11, seqB);
    endWrite(TASK, "title", seqB);
    expect(getCanonicalValue(TASK, "title")).toBe("B-title");
    expect(seen).toEqual([]);

    // A's response arrives second, carrying a later updated_at (it was
    // persisted after B on the server) and releases the guard.
    recordWriteSuccess(TASK, "title", "A-title", T12, seqA);
    endWrite(TASK, "title", seqA);

    expect(getCanonicalValue(TASK, "title")).toBe("A-title");
    expect(seen).toEqual(["A-title"]);
  });

  it("converges on the same later-persisted title regardless of which response is recorded first", () => {
    seedInitialCanonical(TASK, "title", "original", T10);

    const seqA = nextTaskSequence(TASK);
    beginWrite(TASK, "title", seqA);
    const seqB = nextTaskSequence(TASK);
    beginWrite(TASK, "title", seqB);

    // This time the earlier-issued commit's response, carrying the earlier
    // updated_at, is recorded first.
    recordWriteSuccess(TASK, "title", "A-title", T11, seqA);
    endWrite(TASK, "title", seqA);
    recordWriteSuccess(TASK, "title", "B-title", T12, seqB);
    endWrite(TASK, "title", seqB);

    expect(getCanonicalValue(TASK, "title")).toBe("B-title");
  });
});

describe("deferred-apply notification (AC-27, AC-50)", () => {
  it("notifies immediately when a candidate is accepted while unguarded", () => {
    const seen: string[] = [];
    subscribeField(TASK, "title", (value) => seen.push(value));
    seedInitialCanonical(TASK, "title", "v1", T10);
    recordRefetchCandidate(TASK, "title", "v2", T11, nextTaskSequence(TASK));
    expect(seen).toEqual(["v1", "v2"]);
  });

  it("defers notification while guarded, then fires once the guard releases", () => {
    const seen: string[] = [];
    seedInitialCanonical(TASK, "title", "v1", T10);
    subscribeField(TASK, "title", (value) => seen.push(value));

    openFieldEditor(TASK, "title");
    recordRefetchCandidate(TASK, "title", "v2", T11, nextTaskSequence(TASK));
    expect(seen).toEqual([]); // guarded: no notification yet

    closeFieldEditor(TASK, "title");
    expect(seen).toEqual(["v2"]);
  });

  it("unsubscribe stops further notifications", () => {
    const seen: string[] = [];
    const unsubscribe = subscribeField(TASK, "title", (value) => seen.push(value));
    seedInitialCanonical(TASK, "title", "v1", T10);
    unsubscribe();
    recordRefetchCandidate(TASK, "title", "v2", T11, nextTaskSequence(TASK));
    expect(seen).toEqual(["v1"]);
  });
});

describe("multi-task isolation", () => {
  it("does not leak canonical state between tasks", () => {
    seedInitialCanonical("task-a", "title", "A", T10);
    seedInitialCanonical("task-b", "title", "B", T10);
    expect(getCanonicalValue("task-a", "title")).toBe("A");
    expect(getCanonicalValue("task-b", "title")).toBe("B");
  });
});
