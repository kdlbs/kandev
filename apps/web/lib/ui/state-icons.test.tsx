import { describe, expect, it } from "vitest";
import { isValidElement, type ReactNode } from "react";
import {
  IconCheck,
  IconCircleCheck,
  IconCircleFilled,
  IconLoader,
  IconLoader2,
  IconMessageQuestion,
  IconShieldQuestion,
} from "@tabler/icons-react";
import {
  getSessionStateIcon,
  getTaskStateIcon,
  isTaskInFlight,
  shouldShowTaskRunningSpinner,
} from "./state-icons";

function iconType(node: ReactNode) {
  if (!isValidElement(node)) throw new Error("Expected React element");
  return node.type;
}

function iconClassName(node: ReactNode): string {
  if (!isValidElement(node)) throw new Error("Expected React element");
  return (node.props as { className?: string }).className ?? "";
}

describe("getTaskStateIcon", () => {
  it("uses the question icon for waiting-for-input task state", () => {
    expect(iconType(getTaskStateIcon("WAITING_FOR_INPUT"))).toBe(IconMessageQuestion);
  });

  it("uses the question icon when there is a pending clarification", () => {
    expect(iconType(getTaskStateIcon("REVIEW", undefined, true))).toBe(IconMessageQuestion);
  });

  it("keeps review task state as the review check without pending clarification", () => {
    expect(iconType(getTaskStateIcon("REVIEW", undefined, false))).toBe(IconCheck);
  });
});

describe("getTaskStateIcon — waiting-for-input variants (§spec:waiting-for-input-parity)", () => {
  //  clarification / plain waiting → message-question (needs me: answer)
  //  permission                    → shield-question (needs me: approve/deny)
  //  Both must read apart from done AND from both running affordances by SHAPE.
  it("uses the shield-question icon for a pending permission prompt", () => {
    expect(iconType(getTaskStateIcon("REVIEW", undefined, false, null, true))).toBe(
      IconShieldQuestion,
    );
  });

  it("lets a pending permission win over a coarse WAITING_FOR_INPUT state (not masked)", () => {
    // A permission prompt often coincides with the coarse WAITING_FOR_INPUT
    // state; the shield must not be hidden behind the generic question icon.
    expect(iconType(getTaskStateIcon("WAITING_FOR_INPUT", undefined, false, null, true))).toBe(
      IconShieldQuestion,
    );
  });

  it("distinguishes both waiting variants from done and from both running affordances by SHAPE", () => {
    const clarification = iconType(getTaskStateIcon("REVIEW", undefined, true));
    const permission = iconType(getTaskStateIcon("REVIEW", undefined, false, null, true));
    const generating = iconType(getTaskStateIcon("IN_PROGRESS", undefined, false, "generating"));
    const background = iconType(getTaskStateIcon("IN_PROGRESS", undefined, false, "background"));
    const done = iconType(getTaskStateIcon("COMPLETED", undefined, false, null));
    for (const running of [generating, background]) {
      expect(clarification).not.toBe(running);
      expect(permission).not.toBe(running);
    }
    expect(clarification).not.toBe(done);
    expect(permission).not.toBe(done);
    expect(clarification).not.toBe(permission);
  });

  it("lets pending permission win over clarification and foreground activity", () => {
    expect(
      iconType(getTaskStateIcon("WAITING_FOR_INPUT", undefined, true, "generating", true)),
    ).toBe(IconShieldQuestion);
  });

  it.each(["generating", "background"] as const)(
    "lets a pending clarification win over %s activity",
    (activity) => {
      expect(
        iconType(getTaskStateIcon("WAITING_FOR_INPUT", undefined, true, activity, false)),
      ).toBe(IconMessageQuestion);
    },
  );

  it("lets generating activity win over a coarse waiting state without pending input", () => {
    expect(iconType(getTaskStateIcon("WAITING_FOR_INPUT", undefined, false, "generating"))).toBe(
      IconLoader2,
    );
  });

  it("lets background activity win over a coarse waiting state without pending input", () => {
    expect(iconType(getTaskStateIcon("WAITING_FOR_INPUT", undefined, false, "background"))).toBe(
      IconLoader,
    );
  });
});

describe("getTaskStateIcon — task-level activity tri-state", () => {
  //  (a) generating → the established running spinner (IconLoader2)
  //  (b) background → a distinct spinner (IconLoader), NEVER the done check
  //  (c) done       → the coarse check (IconCheck)
  it("(a) generating shows the running spinner even when the coarse state is done", () => {
    // Most-active-wins: a generating session outranks a finished primary that
    // would otherwise render the done check.
    expect(iconType(getTaskStateIcon("COMPLETED", undefined, false, "generating"))).toBe(
      IconLoader2,
    );
  });

  it("(b) background shows a working spinner — never the done check — over a done coarse state", () => {
    const bg = getTaskStateIcon("COMPLETED", undefined, false, "background");
    expect(iconType(bg)).toBe(IconLoader);
    expect(iconType(bg)).not.toBe(IconCheck);
  });

  it("(c) falls through to the coarse task state when no session is active", () => {
    expect(iconType(getTaskStateIcon("COMPLETED", undefined, false, null))).toBe(IconCheck);
    expect(iconType(getTaskStateIcon("COMPLETED", undefined, false, undefined))).toBe(IconCheck);
  });

  it("safe fallback: an in-progress task with a MISSING aggregate reads not-done, never a check", () => {
    // §spec:live-propagation-fallback safe default: a task whose turn is still
    // open (coarse IN_PROGRESS) but whose task-level aggregate is unknown — e.g.
    // the aggregate never reached this client, or the in-memory tracker reset on
    // a backend restart — must fall back to the working spinner, never the done
    // check. The coarse IN_PROGRESS reading is itself not-done, so a missing
    // aggregate can only ever soften to working, never harden to done.
    const missingUndefined = getTaskStateIcon("IN_PROGRESS", undefined, false, undefined);
    const missingNull = getTaskStateIcon("IN_PROGRESS", undefined, false, null);
    expect(iconType(missingUndefined)).toBe(IconLoader2);
    expect(iconType(missingUndefined)).not.toBe(IconCheck);
    expect(iconType(missingNull)).toBe(IconLoader2);
    expect(iconType(missingNull)).not.toBe(IconCheck);
  });

  it("distinguishes background from BOTH generating and done by icon SHAPE, not hue alone", () => {
    // Icon TYPE (glyph) differs for all three, so the reading survives a
    // grayscale/desaturated scan for color-vision-deficient operators
    // (§req:not-color-alone).
    const generating = iconType(getTaskStateIcon("IN_PROGRESS", undefined, false, "generating"));
    const background = iconType(getTaskStateIcon("IN_PROGRESS", undefined, false, "background"));
    const done = iconType(getTaskStateIcon("COMPLETED", undefined, false, null));
    expect(background).not.toBe(generating);
    expect(background).not.toBe(done);
    expect(generating).not.toBe(done);
  });

  it("also separates background from generating and done by HUE on the compact surfaces", () => {
    // The dense board/list/graph surfaces get an extra hue separation on top of the
    // shape difference so background reads apart from generating at a glance — its
    // own violet, distinct from generating's blue and done's green.
    const generating = iconClassName(
      getTaskStateIcon("IN_PROGRESS", undefined, false, "generating"),
    );
    const background = iconClassName(
      getTaskStateIcon("IN_PROGRESS", undefined, false, "background"),
    );
    const done = iconClassName(getTaskStateIcon("COMPLETED", undefined, false, null));
    expect(background).toContain("text-violet-500");
    expect(background).not.toBe(generating);
    expect(background).not.toBe(done);
  });
});

describe("getSessionStateIcon — fine-grained busy tri-state", () => {
  // ADR-0049. Three distinguishable conditions:
  //  (a) RUNNING + generating  → the established static "running" dot (unchanged)
  //  (b) settled + background   → working-in-background spinner, NOT the done check
  //  (c) COMPLETED              → done checkmark
  it("(a) keeps the established static running dot while the foreground is generating", () => {
    // The fine-grained signal only ADDS a background indicator; the foreground
    // running affordance is deliberately left as it always was (static dot).
    const a = getSessionStateIcon("RUNNING", undefined, "generating");
    expect(iconType(a)).toBe(IconCircleFilled);
    expect(iconClassName(a)).not.toContain("animate-spin");
  });

  it("(a) defaults to the running dot when the substate is unknown", () => {
    // Absent/null substate must preserve the historical RUNNING affordance.
    expect(iconType(getSessionStateIcon("RUNNING"))).toBe(IconCircleFilled);
    expect(iconType(getSessionStateIcon("RUNNING", undefined, null))).toBe(IconCircleFilled);
  });

  it("(b) shows a working spinner — never the done checkmark — while background work runs", () => {
    const b = getSessionStateIcon("WAITING_FOR_INPUT", undefined, "background");
    expect(iconType(b)).toBe(IconLoader2);
    expect(iconType(b)).not.toBe(IconCircleCheck);
    expect(iconClassName(b)).toContain("animate-spin");
  });

  it("(b) is visually distinct from (a) so the operator can tell them apart", () => {
    const a = iconClassName(getSessionStateIcon("RUNNING", undefined, "generating"));
    const b = iconClassName(getSessionStateIcon("RUNNING", undefined, "background"));
    expect(a).not.toBe(b);
  });

  it("(c) flips to the done checkmark once background activity is cleared", () => {
    expect(iconType(getSessionStateIcon("COMPLETED"))).toBe(IconCircleCheck);
    // A stale "background" substate must not resurrect a spinner on a terminal
    // session — the coarse state governs (c).
    expect(iconType(getSessionStateIcon("COMPLETED", undefined, "background"))).toBe(
      IconCircleCheck,
    );
  });

  it("distinguishes background-running from BOTH generating and done by icon SHAPE, not hue alone", () => {
    // §req:not-color-alone: the three affordances must be separable in a
    // grayscale/desaturated scan. Asserting the icon *component* (shape) differs
    // — independent of className/hue — guarantees the distinction survives for
    // color-vision-deficient operators. This locks getSessionStateIcon as the
    // single source every session surface calls for all three states.
    const generating = iconType(getSessionStateIcon("RUNNING", undefined, "generating"));
    const background = iconType(getSessionStateIcon("RUNNING", undefined, "background"));
    const done = iconType(getSessionStateIcon("COMPLETED"));
    expect(background).not.toBe(generating);
    expect(background).not.toBe(done);
    expect(generating).not.toBe(done);
  });
});

describe("getSessionStateIcon — waiting-for-input variants (§spec:waiting-for-input-parity)", () => {
  it("reads a plain WAITING_FOR_INPUT session as the needs-me question, not a muted clock", () => {
    // Matches the sidebar: a finished turn awaiting a reply reads as "needs me".
    expect(iconType(getSessionStateIcon("WAITING_FOR_INPUT"))).toBe(IconMessageQuestion);
  });

  it("uses the question icon for a pending clarification even while coarsely RUNNING", () => {
    // The agent stopped mid-turn to ask; the coarse state can still be RUNNING.
    expect(iconType(getSessionStateIcon("RUNNING", undefined, null, true, false))).toBe(
      IconMessageQuestion,
    );
  });

  it("uses the shield icon for a pending permission, taking precedence over clarification", () => {
    expect(iconType(getSessionStateIcon("WAITING_FOR_INPUT", undefined, null, true, true))).toBe(
      IconShieldQuestion,
    );
  });

  it.each(["generating", "background"] as const)(
    "lets a pending clarification win over %s activity",
    (activity) => {
      expect(iconType(getSessionStateIcon("RUNNING", undefined, activity, true, false))).toBe(
        IconMessageQuestion,
      );
    },
  );

  it("lets pending permission win over clarification and background activity", () => {
    expect(
      iconType(getSessionStateIcon("WAITING_FOR_INPUT", undefined, "background", true, true)),
    ).toBe(IconShieldQuestion);
  });

  it("does not let stale pending input mask starting or terminal session states", () => {
    expect(iconType(getSessionStateIcon("STARTING", undefined, "background", true, true))).toBe(
      IconLoader2,
    );
    expect(iconType(getSessionStateIcon("COMPLETED", undefined, "generating", true, true))).toBe(
      IconCircleCheck,
    );
  });

  it("distinguishes both waiting variants from done and from both running affordances by SHAPE", () => {
    const clarification = iconType(getSessionStateIcon("WAITING_FOR_INPUT", undefined, null, true));
    const permission = iconType(
      getSessionStateIcon("WAITING_FOR_INPUT", undefined, null, false, true),
    );
    const generating = iconType(getSessionStateIcon("RUNNING", undefined, "generating"));
    const background = iconType(getSessionStateIcon("RUNNING", undefined, "background"));
    const done = iconType(getSessionStateIcon("COMPLETED"));
    for (const running of [generating, background]) {
      expect(clarification).not.toBe(running);
      expect(permission).not.toBe(running);
    }
    expect(clarification).not.toBe(done);
    expect(permission).not.toBe(done);
    expect(clarification).not.toBe(permission);
  });
});

describe("shouldShowTaskRunningSpinner", () => {
  it("returns false for non-loading task states without an active session", () => {
    expect(shouldShowTaskRunningSpinner("COMPLETED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("FAILED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("CANCELLED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("REVIEW")).toBe(false);
    expect(shouldShowTaskRunningSpinner("TODO")).toBe(false);
  });

  it("returns true for non-TODO task states with an actively running primary session", () => {
    expect(shouldShowTaskRunningSpinner("REVIEW", "RUNNING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("COMPLETED", "RUNNING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("FAILED", "RUNNING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("CANCELLED", "RUNNING")).toBe(true);
  });

  it("returns true for SCHEDULING with no primary session yet", () => {
    expect(shouldShowTaskRunningSpinner("SCHEDULING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("SCHEDULING", null)).toBe(true);
    expect(shouldShowTaskRunningSpinner("SCHEDULING", undefined)).toBe(true);
  });

  it("returns true for IN_PROGRESS when the primary session is actively running", () => {
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "RUNNING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "STARTING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "CREATED")).toBe(true);
  });

  it("returns true for IN_PROGRESS when no primary session is attached yet", () => {
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", undefined)).toBe(true);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", null)).toBe(true);
  });

  it("suppresses the spinner when the primary session is terminal", () => {
    // Repro from issue #985: agent finishes (session → COMPLETED) but the
    // workflow leaves the task in IN_PROGRESS for review/manual move. The
    // spinner must not keep spinning forever.
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "COMPLETED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "FAILED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "CANCELLED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("SCHEDULING", "COMPLETED")).toBe(false);
  });

  it("suppresses the spinner when the primary session is paused (waiting/idle)", () => {
    // Same desync class, paused branch: agent stopped to wait for input or
    // was torn down (office IDLE). The spinner is misleading.
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "WAITING_FOR_INPUT")).toBe(false);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "IDLE")).toBe(false);
  });

  it("suppresses the spinner for a CREATED primary session on an inactive task", () => {
    // Repro from the stuck kanban cards (PR #11571 / #11502): task CREATED,
    // sitting in a Waiting column, with a primary session that was persisted in
    // CREATED and never advanced (no executor, no turns). CREATED means "agent
    // not started", so it must defer to the task state instead of spinning.
    expect(shouldShowTaskRunningSpinner("CREATED", "CREATED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("REVIEW", "CREATED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("COMPLETED", "CREATED")).toBe(false);
  });

  it("still spins for a CREATED primary session during a genuine launch", () => {
    // During an actual launch the task state is SCHEDULING/IN_PROGRESS while the
    // session momentarily sits in CREATED. Deferring to the task state keeps the
    // spinner on for that startup window.
    expect(shouldShowTaskRunningSpinner("SCHEDULING", "CREATED")).toBe(true);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "CREATED")).toBe(true);
  });

  it("suppresses the spinner for TODO regardless of primary session state", () => {
    // TODO is the queued/not-started column. A stale primary session state
    // (e.g. task moved back from IN_PROGRESS with the session still alive)
    // must not paint the running spinner on the kanban card.
    expect(shouldShowTaskRunningSpinner("TODO", "RUNNING")).toBe(false);
    expect(shouldShowTaskRunningSpinner("TODO", "STARTING")).toBe(false);
    expect(shouldShowTaskRunningSpinner("TODO", "CREATED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("TODO", "COMPLETED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("TODO", "WAITING_FOR_INPUT")).toBe(false);
    expect(shouldShowTaskRunningSpinner("TODO", "IDLE")).toBe(false);
  });
});

describe("isTaskInFlight", () => {
  // The destructive-action guard (§spec:destructive-action-guard) reads the same
  // task-level foreground_activity aggregate the board indicators show:
  // generating OR background-running ⇒ still working. Sharing this derivation with
  // getTaskStateIconConfig keeps the archive/delete warning in lockstep with the
  // card's busy affordance — the guard can never disagree with what the operator sees.
  it("reports in-flight while the task is generating", () => {
    expect(isTaskInFlight("generating")).toBe(true);
  });

  it("reports in-flight while spawned background work is running", () => {
    expect(isTaskInFlight("background")).toBe(true);
  });

  it("reports idle when there is no foreground activity", () => {
    expect(isTaskInFlight(null)).toBe(false);
    expect(isTaskInFlight(undefined)).toBe(false);
  });
});
