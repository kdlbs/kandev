import { describeSchedule } from "@/components/automations/schedule-expression";
import type { Automation, AutomationRun, AutomationSummary } from "@/lib/types/automation";
import { formatNextFiring, nextFiringInstant, scheduleBinding } from "./automation-schedule";
import { t } from "@/lib/i18n";

/**
 * What the row's dot says about health. Deliberately three words, not the run
 * status vocabulary: the question a list row answers is "is this thing okay",
 * which a per-run status cannot answer on its own.
 */
export type AutomationActivityState = "running" | "idle" | "paused";

/**
 * The next-run cell is one of two things and never both: a resolved time, or
 * the reason there will not be one. Modelling it as a union stops a caller
 * printing a time that will not happen next to an explanation of why it won't.
 */
export type NextFiring = { kind: "time"; text: string } | { kind: "reason"; text: string };

// Catalog keys, not copy: these are module-scope constants, so a `t()` here
// would pin the reason text to the boot locale. Callers resolve them.
export const AUTOMATION_OFF_REASON_KEY = "automations:nextFiringSwitchedOff";
export const NO_SCHEDULE_REASON_KEY = "automations:nextFiringNoSchedule";
export const SCHEDULE_OFF_REASON_KEY = "automations:nextFiringScheduleOff";

/**
 * The concurrency cap states its own number. "Paused" alone reads as something
 * the user did; naming the cap points at the setting that would change it.
 */
export function concurrencyReason(maxConcurrentRuns: number): string {
  return t("automations:nextFiringConcurrencyPaused", { max: Math.max(1, maxConcurrentRuns) });
}

export const STATE_DOT_CLASS: Record<AutomationActivityState, string> = {
  running: "bg-blue-500",
  idle: "bg-emerald-500",
  paused: "bg-muted-foreground/50",
};

/**
 * Catalog keys rather than copy: this is a plain module, so the label has to be
 * resolved by whoever renders it and has a `t`.
 */
export const STATE_LABEL_KEY: Record<AutomationActivityState, string> = {
  running: "automations:stateRunning",
  idle: "automations:stateIdle",
  paused: "automations:statePaused",
};

/**
 * Whether the cap is genuinely holding a firing back in a way worth saying.
 *
 * A single-slot automation with one run open is the ordinary steady state of
 * every run it will ever do — the moment it starts, it is "at the cap". Calling
 * that Paused made the normal case look broken, and it said it in amber right
 * next to a dot already reporting `running`. The cap is only news once it is
 * queueing something: more than one slot configured, and every slot taken.
 *
 * Known and deliberate: `max=2/open=2` is just as much "at capacity" as
 * `max=1/open=1`, so the rule is not symmetric, and `openRuns` is not evidence
 * that anything is actually queued behind the cap. The honest version warns
 * only when a firing was really turned away — those runs already exist, with a
 * reason, as `skipped` rows — but the summary this derivation reads carries no
 * skip count, so saying so would take a new backend field. Until then this
 * errs toward silence in the one case that is always benign, rather than
 * toward an amber note on every healthy single-slot automation.
 */
function atConcurrencyCap(automation: Automation, openRuns: number): boolean {
  const max = Math.max(1, automation.max_concurrent_runs);
  if (max === 1 && openRuns <= 1) return false;
  return openRuns >= max;
}

/** A time when one can be resolved, otherwise the schedule described in words. */
function scheduleWindow(automation: Automation, now: Date): NextFiring {
  const { expression, timeZone } = scheduleBinding(automation);
  const at = nextFiringInstant(automation, now);
  // An `@every` interval is anchored server-side and an unrecognised expression
  // cannot be resolved at all, so state the rule rather than invent an instant.
  if (!at) return { kind: "time", text: describeSchedule(expression, timeZone) };
  return { kind: "time", text: formatNextFiring(at, timeZone, now) };
}

/**
 * When it fires next, or why it will not. Ordered by which fact makes the
 * others moot: a disabled automation's schedule is irrelevant, and a schedule
 * that does not exist cannot be blocked by concurrency.
 */
export function nextFiring(
  automation: Automation,
  openRuns: number,
  now: Date = new Date(),
): NextFiring {
  if (!automation.enabled) return { kind: "reason", text: t(AUTOMATION_OFF_REASON_KEY) };
  const { trigger, expression } = scheduleBinding(automation);
  if (!trigger || !expression) return { kind: "reason", text: t(NO_SCHEDULE_REASON_KEY) };
  if (!trigger.enabled) return { kind: "reason", text: t(SCHEDULE_OFF_REASON_KEY) };
  if (atConcurrencyCap(automation, openRuns)) {
    return { kind: "reason", text: concurrencyReason(automation.max_concurrent_runs) };
  }
  return scheduleWindow(automation, now);
}

/** Whether anything is standing between this automation and its next firing. */
function firesAgain(automation: Automation, openRuns: number): boolean {
  return nextFiring(automation, openRuns).kind === "time";
}

/**
 * Health, which is not the same question as "will it fire". An automation held
 * back by its own concurrency cap is `running` — something IS happening — while
 * its next-run cell explains why nothing new will start.
 */
export function automationState(automation: Automation, openRuns: number): AutomationActivityState {
  if (openRuns > 0) return "running";
  if (!automation.enabled) return "paused";
  const { trigger, expression } = scheduleBinding(automation);
  if (trigger && expression && !trigger.enabled) return "paused";
  return "idle";
}

export type AutomationRow = {
  automation: Automation;
  state: AutomationActivityState;
  next: NextFiring;
  lastRun: AutomationRun | null;
  /** Epoch ms of the most recent thing that happened, used only for ordering. */
  lastActivity: number;
  /**
   * Epoch ms of the next firing, or null when there will not be one. Separate
   * from `next.text` because an agenda has to sort by the instant, and a
   * formatted string sorts alphabetically — "~09:00 tomorrow" before "~23:00
   * today".
   */
  nextAt: number | null;
};

function timestamp(value: string | null | undefined): number {
  if (!value) return 0;
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? 0 : parsed;
}

function lastActivityOf(automation: Automation, lastRun: AutomationRun | null): number {
  // An automation that has never run still has to sort somewhere. Its creation
  // time puts a brand-new one near the top, where the user just left it.
  return Math.max(
    timestamp(lastRun?.created_at),
    timestamp(automation.last_triggered_at),
    timestamp(automation.created_at),
  );
}

/**
 * One row per automation, most recently active first.
 *
 * Health comes from the server's per-automation summary, not from scanning the
 * workspace feed: the feed is capped, so a quiet automation behind a noisy one
 * would report "No runs yet" and idle — the two claims a health row must never
 * get wrong. An automation with no summary has genuinely never run.
 */
export function buildAutomationRows(
  automations: Automation[],
  summaries: AutomationSummary[],
  now: Date = new Date(),
): AutomationRow[] {
  const byAutomation = new Map(summaries.map((summary) => [summary.automation_id, summary]));
  const rows = automations.map((automation) => {
    const summary = byAutomation.get(automation.id);
    const openRuns = summary?.open_runs ?? 0;
    const lastRun = summary?.last_run ?? null;
    return {
      automation,
      state: automationState(automation, openRuns),
      next: nextFiring(automation, openRuns, now),
      lastRun,
      lastActivity: lastActivityOf(automation, lastRun),
      nextAt: firesAgain(automation, openRuns)
        ? (nextFiringInstant(automation, now)?.getTime() ?? null)
        : null,
    };
  });
  return rows.sort(
    (a, b) => b.lastActivity - a.lastActivity || a.automation.name.localeCompare(b.automation.name),
  );
}

/**
 * The forward agenda: what fires next, soonest first.
 *
 * This is the one question neither the sidebar nor an automation's own page can
 * answer — the sidebar has no room for a time, and a detail page knows only
 * about itself. Automations that will not fire sort last, holding their reason
 * rather than being dropped: "nothing is scheduled" is an answer the reader
 * needs, and hiding them would make the agenda look complete when it is not.
 */
export function buildAgenda(
  automations: Automation[],
  summaries: AutomationSummary[],
  now: Date = new Date(),
): AutomationRow[] {
  return buildAutomationRows(automations, summaries, now).sort((a, b) => {
    if (a.nextAt !== null && b.nextAt !== null) return a.nextAt - b.nextAt;
    if (a.nextAt !== null) return -1;
    if (b.nextAt !== null) return 1;
    return a.automation.name.localeCompare(b.automation.name);
  });
}
