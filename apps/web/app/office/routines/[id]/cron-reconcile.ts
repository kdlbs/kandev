import {
  createRoutineTrigger,
  deleteRoutineTrigger,
  listRoutineTriggers,
} from "@/lib/api/domains/office-api";
import type { RoutineTrigger } from "@/lib/state/slices/office/types";

export type CronDraft = {
  triggerKind: string;
  cronExpression: string;
  timezone: string;
};

export type CronReconcileOutcome =
  | { kind: "unchanged" }
  | { kind: "success"; triggers: RoutineTrigger[] | null }
  | { kind: "create-failed"; message: string; triggers: RoutineTrigger[] | null }
  | { kind: "delete-failed"; message: string; triggers: RoutineTrigger[] | null };

function effectiveTimezone(timezone: string | undefined): string {
  const trimmed = timezone?.trim();
  return trimmed ? trimmed : "UTC";
}

function cronMatches(existing: RoutineTrigger, cronExpression: string, timezone: string): boolean {
  return (
    existing.cronExpression === cronExpression &&
    effectiveTimezone(existing.timezone) === effectiveTimezone(timezone)
  );
}

async function relistOrNull(routineId: string): Promise<RoutineTrigger[] | null> {
  try {
    const res = await listRoutineTriggers(routineId);
    return res.triggers;
  } catch {
    return null;
  }
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

/**
 * Reconciles the routine's cron trigger with the draft's expression, timezone,
 * and kind. Creates the replacement trigger before deleting the old one, and
 * always re-lists from the server afterward rather than splicing the local
 * array, per docs/specs/office/system-design/routine-wire-contract.md.
 */
export async function reconcileCronTrigger(
  routineId: string,
  draft: CronDraft,
  triggers: RoutineTrigger[],
): Promise<CronReconcileOutcome> {
  if (draft.triggerKind !== "cron") return { kind: "unchanged" };
  const cronExpression = draft.cronExpression.trim();
  if (!cronExpression) return { kind: "unchanged" };

  const existing = triggers.find((t) => t.kind === "cron") ?? null;
  if (existing && cronMatches(existing, cronExpression, draft.timezone)) {
    return { kind: "unchanged" };
  }

  try {
    await createRoutineTrigger(routineId, {
      kind: "cron",
      cronExpression,
      timezone: draft.timezone,
    });
  } catch (err) {
    return {
      kind: "create-failed",
      message: errorMessage(err),
      triggers: await relistOrNull(routineId),
    };
  }

  if (existing) {
    try {
      await deleteRoutineTrigger(existing.id);
    } catch (err) {
      return {
        kind: "delete-failed",
        message: errorMessage(err),
        triggers: await relistOrNull(routineId),
      };
    }
  }

  return { kind: "success", triggers: await relistOrNull(routineId) };
}
