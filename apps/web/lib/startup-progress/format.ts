import type { TFunction } from "i18next";
import { estimateDurationComponents } from "./duration";
import type { StartupStepSnapshot } from "./types";

const STARTUP_KEY_PREFIX = "startup.";

/** Strips the backend's "startup." key prefix so it can be looked up in the web's `startup` i18next namespace. */
function stripStartupPrefix(key: string): string {
  return key.startsWith(STARTUP_KEY_PREFIX) ? key.slice(STARTUP_KEY_PREFIX.length) : key;
}

export function formatDuration(t: TFunction, ms: number): string {
  const { value, minutes } = estimateDurationComponents(ms);
  const key = minutes ? "startup:duration.minutes" : "startup:duration.seconds";
  return t(key, { count: value });
}

export function formatElapsed(t: TFunction, ms: number): string {
  return t("startup:page.elapsed", { duration: formatDuration(t, ms) });
}

export function formatEta(t: TFunction, etaMs: number): string {
  return t("startup:page.eta", { duration: formatDuration(t, etaMs) });
}

export function formatStalled(t: TFunction, sinceAdvanceMs: number): string {
  return t("startup:page.stalled", { duration: formatDuration(t, sinceAdvanceMs) });
}

export function formatPhaseLabel(t: TFunction, phase: string): string {
  return t(`startup:phase.${phase}`);
}

export function formatStepLabel(t: TFunction, step: StartupStepSnapshot): string {
  return t(`startup:${stripStartupPrefix(step.label_key)}`);
}

function unitLabel(t: TFunction, unit: string, count: number): string {
  return t(`startup:unit.${unit}`, { count });
}

/**
 * Mirrors the Go page's countingProgressText/applyCountedProgress
 * (AC-PLATFORM-STARTUP-PROGRESS-001.4/001.5/001.6, AC-003.8): opaque steps
 * report no count, counting steps pluralize the unit off `done`, counted
 * steps pluralize off `total`.
 */
export function formatStepProgress(t: TFunction, step: StartupStepSnapshot): string {
  if (step.measure === "opaque") return t("startup:page.progressUnavailable");
  if (step.measure === "counting") {
    const done = step.done ?? 0;
    return t("startup:page.countingProgress", { done, unit: unitLabel(t, step.unit, done) });
  }
  const done = step.done ?? 0;
  const total = step.total ?? 0;
  return t("startup:page.countedProgress", { done, total, unit: unitLabel(t, step.unit, total) });
}

/** Mirrors Go's percentDone (AC-PLATFORM-STARTUP-PROGRESS-003.9): clamped to [0, 100]. */
export function percentDone(done: number, total: number): number {
  if (total <= 0) return 0;
  const pct = (done / total) * 100;
  if (pct < 0) return 0;
  if (pct > 100) return 100;
  return pct;
}
