import { notFound } from "next/navigation";
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
    const [routineRes, triggersRes] = await Promise.all([
      getRoutine(id, { cache: "no-store" }),
      listRoutineTriggers(id, { cache: "no-store" }).catch(() => ({
        triggers: [] as RoutineTrigger[],
      })),
    ]);
    routine =
      (routineRes as unknown as { routine?: Routine }).routine ??
      (routineRes as unknown as Routine);
    triggers = triggersRes.triggers ?? [];
  } catch {
    routine = null;
  }
  if (!routine) notFound();
  return <RoutineDetailView initialRoutine={routine} initialTriggers={triggers} />;
}
