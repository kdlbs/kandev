/**
 * Copy the jsx-only guard cannot see.
 *
 * Every `it` below that names a shape is a string this check found in already
 * migrated code — code that `pnpm lint`, `i18n:check`, the new-code ratchet and
 * a pseudo-locale walk all reported clean. The point of the file is not that
 * the detector fires, it is that each shape is a DISTINCT hole: a test suite
 * asserting only "finds a hardcoded label" would have passed against three
 * successive versions of this detector that each missed something real.
 *
 * Four of these are regressions against the detector's own first drafts, and are
 * marked as such. They were not found by reasoning about the code; they were
 * found by pointing it at the repository and reading what it did not say.
 */
import { describe, expect, it } from "vitest";

import { findNonJsxCopy, looksLikeCopy } from "./nonjsx-copy.mjs";

/** Stand-in copy, reused so the fixtures differ only in the shape under test. */
const COPY = "Disk usage";

/** The values reported for `source`, in order. */
function copyIn(source: string, excludes: string[] = []) {
  return findNonJsxCopy(source, { filename: "sample.tsx", excludes }).findings.map((f) => f.value);
}

function kindsIn(source: string) {
  return findNonJsxCopy(source, { filename: "sample.tsx" }).findings.map((f) => f.kind);
}

describe("looksLikeCopy", () => {
  it("accepts a sentence and a capitalized word", () => {
    expect(looksLikeCopy("Waiting for input")).toBe(true);
    expect(looksLikeCopy("Staged")).toBe(true);
  });

  it("rejects identifiers, enum values, paths and urls", () => {
    for (const value of [
      "waitingForInput",
      "waiting_for_input",
      "WAITING",
      "task:panelAgent",
      "./relative/path",
      "https://example.com/x",
    ]) {
      expect(looksLikeCopy(value), value).toBe(false);
    }
  });

  it("rejects Tailwind class lists, which are long and word-like", () => {
    for (const value of [
      "flex items-center gap-2",
      "h-6 gap-1 px-2.5 text-xs cursor-pointer",
      "text-muted-foreground",
    ]) {
      expect(looksLikeCopy(value), value).toBe(false);
    }
  });

  it("keeps lowercase prose that an all-lowercase filter would have eaten", () => {
    // REGRESSION: the first draft rejected every all-lowercase multi-word
    // string as a class list. That silently dropped four schedule labels and a
    // monitor status, all of them rendered.
    expect(looksLikeCopy("every hour")).toBe(true);
    expect(looksLikeCopy("ended (session restart)")).toBe(true);
  });

  it("defers to the eslint guard's own exclusions for brand nouns", () => {
    expect(looksLikeCopy("Azure DevOps")).toBe(true);
    expect(looksLikeCopy("Azure DevOps", ["(Kandev|GitHub|Azure DevOps)"])).toBe(false);
  });

  it("does not backtrack on a long run of hyphens", () => {
    // REGRESSION (CodeQL): the class-list test was one regex nesting a
    // quantifier inside a quantifier over an alphabet containing `-`, so
    // `"a" + "-".repeat(n)` had exponentially many parses — measured at 1ms for
    // n=24 and 51ms for n=32, i.e. minutes by n=50. It is now checked token by
    // token, which is linear. This gate runs over ~1,400 files and in
    // pre-commit, so a hang here stops commits rather than merely being slow.
    // The trailing "!" is load-bearing: it is what makes the match FAIL after
    // the engine has explored every split of the hyphen run. Without it the old
    // regex matched on the first try and returned in microseconds, so a probe
    // built from hyphens alone passes against the very pattern it should catch.
    for (const input of ["a".concat("-".repeat(40), "!"), "a-- a".concat("-".repeat(40), "!")]) {
      const started = performance.now();
      looksLikeCopy(input);
      expect(performance.now() - started).toBeLessThan(50);
    }
  });
});

describe("findNonJsxCopy", () => {
  it("finds a label in a SCREAMING_CASE table, which eslint skips entirely", () => {
    const source = `const ROWS = [{ id: "disk", label: "Disk usage" }];`;
    expect(copyIn(source)).toEqual([COPY]);
    expect(kindsIn(source)).toEqual(["config-table"]);
  });

  it("finds copy in a table keyed by an enum, not by a copy-shaped name", () => {
    // REGRESSION: an earlier draft only inspected slots called `label`/`title`,
    // so a lookup table read its key ("every-5m"), called the slot data, and
    // skipped the sentence inside it.
    const source = `const INTERVAL_LABELS = { "every-5m": "Every 5 minutes" };`;
    expect(copyIn(source)).toEqual(["Every 5 minutes"]);
  });

  it("finds a string returned from a plain helper", () => {
    const source = `function badge(x) { if (x) return "Waiting for input"; return null; }`;
    expect(copyIn(source)).toEqual(["Waiting for input"]);
    expect(kindsIn(source)).toEqual(["helper-return"]);
  });

  it("finds a default in a destructuring parameter", () => {
    const source = `function C({ label = "Workspace not available" }) { return label; }`;
    expect(copyIn(source)).toEqual(["Workspace not available"]);
    expect(kindsIn(source)).toEqual(["param-default"]);
  });

  it("finds a toast title", () => {
    const source = `function f() { toast({ title: "Rename failed" }); }`;
    expect(copyIn(source)).toEqual(["Rename failed"]);
  });

  it("finds a state setter named for its own screen", () => {
    // REGRESSION: the first draft enumerated setter names. `setPreviewError`
    // was not on the list, and sat two lines above one that was.
    const source = `function f() { setPreviewError("This invite link is missing its token."); }`;
    expect(kindsIn(source)).toEqual(["display-call"]);
  });

  it("finds a setter reached through an object", () => {
    // REGRESSION: matching only the dotted callee name missed
    // `setters.setError(...)`, which is how a setter bag is passed down.
    const source = `function f() { setters.setError("Failed to resume session"); }`;
    expect(copyIn(source)).toEqual(["Failed to resume session"]);
  });

  it("finds copy inside a template literal", () => {
    const source = "function d(at, zone) { return `Every day at ${at} ${zone}`; }";
    expect(copyIn(source)).toEqual(["Every day at ${…} ${…}"]);
  });

  it("does not read a template's chunks as one phrase", () => {
    // Joining the static chunks would splice "px" and "em" into a two-word
    // phrase and report a CSS value as a sentence.
    expect(copyIn("function s(w, h) { return `${w}px ${h}em`; }")).toEqual([]);
  });

  it("leaves JSX to the eslint rule rather than double-reporting it", () => {
    expect(
      copyIn(`function C() { return <span title="Archive board">Archive board</span>; }`),
    ).toEqual([]);
  });

  it("ignores a component's className ternary", () => {
    const source = `function rowClassName(active) { return active ? "bg-muted text-foreground" : "text-muted-foreground"; }`;
    expect(copyIn(source)).toEqual([]);
  });

  it("ignores a type, an import and an object key", () => {
    const source = [
      `import { x } from "./some module";`,
      `type Mode = "Read only";`,
      `const m = { "Disk usage": 1 };`,
    ].join("\n");
    expect(copyIn(source)).toEqual([]);
  });
});

describe("i18n-exempt markers", () => {
  it("silences the statement they sit above", () => {
    const source = [
      `// i18n-exempt: persisted preset seed.`,
      `const PRESETS = [{ label: "New action" }];`,
    ].join("\n");
    expect(copyIn(source)).toEqual([]);
  });

  it("silences a whole helper when placed above the function", () => {
    // A prompt builder is one decision with several return statements; requiring
    // a marker on each invites whoever adds the next one to skip it.
    const source = [
      `// i18n-exempt: sent verbatim to the agent.`,
      `function buildPrompt(a, b) {`,
      `  if (a) return "Comment from someone on a file:";`,
      `  return "Comment from someone:";`,
      `}`,
    ].join("\n");
    expect(copyIn(source)).toEqual([]);
  });

  it("does not leak from one statement to a later one in the same function", () => {
    // REGRESSION (review of #2711): the marker used to attach by LINE RANGE —
    // every line from an enclosing target's start down to the finding — so a
    // marker on one statement inside a function silenced every literal after it
    // in that function. Attachment is now by Babel's `leadingComments`, which
    // binds a comment to exactly the node it precedes.
    const source = [
      `function build(flag) {`,
      `  // i18n-exempt: persisted preset seed.`,
      `  const seeded = "New action";`,
      `  if (flag) return "Visible user label";`,
      `  return seeded;`,
      `}`,
    ].join("\n");
    expect(copyIn(source)).toEqual(["Visible user label"]);
  });

  it("silences one row of a config table without covering its siblings", () => {
    const source = [
      `const ROWS = [`,
      `  // i18n-exempt: directory name on disk.`,
      `  { label: "Pods" },`,
      `  { label: "Disk usage" },`,
      `];`,
    ].join("\n");
    expect(copyIn(source)).toEqual([COPY]);
  });

  it("does not silence the next statement as well", () => {
    const source = [
      `// i18n-exempt: persisted preset seed.`,
      `const PRESETS = [{ label: "New action" }];`,
      `const HEADERS = [{ label: "Disk usage" }];`,
    ].join("\n");
    expect(copyIn(source)).toEqual([COPY]);
  });

  it("rejects a marker with no reason instead of honouring it", () => {
    // An unexplained exemption is the thing this check exists to prevent: the
    // shapes it finds all have a legitimate form, and the reason is how the
    // next reader tells them apart.
    const source = [`// i18n-exempt`, `const ROWS = [{ label: "Disk usage" }];`].join("\n");
    const result = findNonJsxCopy(source, { filename: "sample.tsx" });
    expect(result.markersWithoutReason).toEqual([1]);
    expect(result.findings.map((f) => f.value)).toEqual([COPY]);
  });
});

describe("robustness", () => {
  it("reports nothing rather than throwing on a file it cannot parse", () => {
    // This runs over ~1,400 files; a gate that dies on one of them reports
    // nothing about the other 1,399 while still exiting 0.
    // The shape must match a clean parse too: every caller destructures
    // `{ findings }`, so a bare array here throws on the NEXT file instead.
    expect(findNonJsxCopy("function ( { unbalanced", { filename: "broken.tsx" })).toEqual({
      findings: [],
      markersWithoutReason: [],
    });
  });
});
