import { useEffect, useState } from "react";
import { RoutineDetailView } from "@/app/office/routines/[id]/routine-detail-view";
import { getRoutine, listRoutineTriggers } from "@/lib/api/domains/office-api";
import type { Routine, RoutineTrigger } from "@/lib/state/slices/office/types";

type LoadState<T> =
  | { status: "loading" }
  | { status: "ready"; data: T }
  | { status: "error"; message: string };

type RoutineDetailData = {
  routine: Routine;
  triggers: RoutineTrigger[];
};

export function RoutineDetailRoute({ routineId }: { routineId: string }) {
  const [state, setState] = useState<LoadState<RoutineDetailData>>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    setState({ status: "loading" });

    async function loadRoutineDetail(): Promise<RoutineDetailData> {
      const [routineResponse, triggersResponse] = await Promise.all([
        getRoutine(routineId, { cache: "no-store" }),
        listRoutineTriggers(routineId, { cache: "no-store" }).catch(() => ({
          triggers: [] as RoutineTrigger[],
        })),
      ]);
      const routine =
        (routineResponse as unknown as { routine?: Routine }).routine ??
        (routineResponse as unknown as Routine);
      return { routine, triggers: triggersResponse.triggers ?? [] };
    }

    loadRoutineDetail()
      .then((data) => {
        if (!cancelled) setState({ status: "ready", data });
      })
      .catch((error: unknown) => {
        if (!cancelled) setState(toErrorState(error));
      });

    return () => {
      cancelled = true;
    };
  }, [routineId]);

  if (state.status !== "ready") {
    return <RoutineRoutePlaceholder state={state} />;
  }

  return (
    <RoutineDetailView initialRoutine={state.data.routine} initialTriggers={state.data.triggers} />
  );
}

function RoutineRoutePlaceholder<T>({ state }: { state: LoadState<T> }) {
  if (state.status === "error") {
    return <div className="py-8 text-sm text-destructive">{state.message}</div>;
  }

  return <div className="py-8 text-sm text-muted-foreground">Loading routine...</div>;
}

function toErrorState(error: unknown): LoadState<never> {
  return {
    status: "error",
    message: error instanceof Error ? error.message : "Failed to load routine",
  };
}
