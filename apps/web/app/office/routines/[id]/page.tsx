import { notFound } from "@/lib/routing/server-navigation";
import { getRoutine, listRoutineTriggers } from "@/lib/api/domains/office-api";
import type { Routine, RoutineTrigger } from "@/lib/state/slices/office/types";
import { RoutineDetailView } from "./routine-detail-view";

type Props = { params: Promise<{ id: string }> };

/**
 * Routine detail / edit page. The server component fetches the routine
 * and its triggers in parallel so the client view renders with no
 * blocking spinner. The client view holds a draft in `useState` and
 * PATCHes to /routines/:id on Save.
 */
export default async function RoutineDetailPage({ params }: Props) {
  const { id } = await params;
  let routine: Routine | null = null;
  let triggers: RoutineTrigger[] = [];
  try {
    // A failed trigger list must not silently become "no trigger exists":
    // the client's save-time reconciliation (cron-reconcile.ts) trusts this
    // initial list to detect an already-armed cron trigger, so swallowing a
    // failure here would make a later Save create a duplicate schedule
    // instead of replacing the one that failed to load. Let it fail the
    // whole page load instead, same as a `getRoutine` failure below.
    const [routineRes, triggersRes] = await Promise.all([
      getRoutine(id, { cache: "no-store" }),
      listRoutineTriggers(id, { cache: "no-store" }),
    ]);
    routine = routineRes;
    triggers = triggersRes.triggers ?? [];
  } catch {
    routine = null;
  }
  if (!routine) notFound();
  return <RoutineDetailView initialRoutine={routine} initialTriggers={triggers} />;
}
