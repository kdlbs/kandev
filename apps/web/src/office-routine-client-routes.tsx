import { useEffect, useState } from "react";
import { RoutineDetailView } from "@/app/office/routines/[id]/routine-detail-view";
import { getRoutine, listRoutineTriggers } from "@/lib/api/domains/office-api";
import { toRouteErrorState, type LoadState } from "@/lib/routing/client-route-helpers";
import type { Routine, RoutineTrigger } from "@/lib/state/slices/office/types";
import { useTranslation } from "react-i18next";

type RoutineDetailData = {
  routine: Routine;
  triggers: RoutineTrigger[];
};

export function RoutineDetailRoute({ routineId }: { routineId: string }) {
  const { t } = useTranslation();
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
        if (!cancelled) setState(toRouteErrorState(error, t("common:failedToLoadRoutine")));
      });

    return () => {
      cancelled = true;
    };
  }, [routineId, t]);

  if (state.status !== "ready") {
    return <RoutineRoutePlaceholder state={state} />;
  }

  return (
    <RoutineDetailView initialRoutine={state.data.routine} initialTriggers={state.data.triggers} />
  );
}

function RoutineRoutePlaceholder<T>({ state }: { state: LoadState<T> }) {
  const { t } = useTranslation();
  if (state.status === "error") {
    return <div className="py-8 text-sm text-destructive">{state.message}</div>;
  }

  return <div className="py-8 text-sm text-muted-foreground">{t("common:loadingRoutine")}</div>;
}
