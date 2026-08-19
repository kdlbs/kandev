"use client";

import { useCallback, useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { IconPlus } from "@tabler/icons-react";
import { toast } from "@/lib/toast/sonner";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfiles } from "@/lib/state/slices/office/selectors";
import {
  listRoutines,
  createRoutine,
  updateRoutine,
  deleteRoutine,
  runRoutine,
  listAllRoutineRuns,
  createRoutineTrigger,
  listRoutineTriggers,
} from "@/lib/api/domains/office-api";
import type {
  Routine,
  AgentProfile,
  RoutineRun,
  RoutineTrigger,
} from "@/lib/state/slices/office/types";
import { RoutineRow } from "./routine-row";
import { RunRow } from "./run-row";
import { CreateRoutineDialog } from "./create-routine-dialog";
import { EmptyState } from "../components/shared/empty-state";
import { useTranslation } from "react-i18next";

type RoutineFormData = {
  name: string;
  description: string;
  taskTitle: string;
  taskDescription: string;
  assigneeAgentProfileId: string;
  concurrencyPolicy: string;
  catchUpPolicy: string;
  catchUpMax: number;
  triggerKind: string;
  cronExpression: string;
  timezone: string;
};

function useRoutineActions(workspaceId: string | null, fetchRoutines: () => Promise<void>) {
  const { t } = useTranslation();
  const handleToggle = useCallback(
    async (id: string, active: boolean) => {
      try {
        await updateRoutine(id, { status: active ? "active" : "paused" } as Record<
          string,
          unknown
        >);
        await fetchRoutines();
        toast.success(active ? t("office:routineActivated") : t("office:routinePaused"));
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t("office:failedToUpdateRoutine"));
      }
    },
    [fetchRoutines],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      try {
        await deleteRoutine(id);
        await fetchRoutines();
        toast.success(t("office:routineDeleted"));
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t("office:failedToDeleteRoutine"));
      }
    },
    [fetchRoutines],
  );

  const handleCreate = useCallback(
    async (data: RoutineFormData, onDone: () => void) => {
      if (!workspaceId) return;
      try {
        const template = JSON.stringify({
          title: data.taskTitle,
          description: data.taskDescription,
        });
        const res = await createRoutine(workspaceId, {
          name: data.name,
          description: data.description,
          taskTemplate: JSON.parse(template),
          assigneeAgentProfileId: data.assigneeAgentProfileId,
          concurrencyPolicy: data.concurrencyPolicy,
          catchUpPolicy: data.catchUpPolicy,
          catchUpMax: data.catchUpMax,
        } as Record<string, unknown>);
        if (data.triggerKind === "cron" && data.cronExpression && res) {
          const routineObj = res as unknown as { routine?: { id: string } };
          const routineId = routineObj.routine?.id ?? (res as unknown as { id: string }).id;
          if (routineId) {
            await createRoutineTrigger(routineId, {
              kind: data.triggerKind as "cron",
              cronExpression: data.cronExpression,
              timezone: data.timezone,
            });
          }
        }
        onDone();
        await fetchRoutines();
        toast.success(t("office:routineCreated"));
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t("office:failedToCreateRoutine"));
      }
    },
    [workspaceId, fetchRoutines],
  );

  return { handleToggle, handleDelete, handleCreate };
}

// useRoutinesData centralises the list / runs / triggers fetch so the
// RoutinesContent component stays under the per-function ceiling. Each
// fetcher is referentially stable (depends on workspaceId only) so the
// effects don't re-fire on unrelated re-renders.
function useRoutinesData(workspaceId: string | null) {
  const routines = useAppStore((s) => s.office.routines);
  const setRoutines = useAppStore((s) => s.setRoutines);

  const [runs, setRuns] = useState<RoutineRun[]>([]);
  const [triggersByRoutine, setTriggersByRoutine] = useState<Record<string, RoutineTrigger[]>>({});

  const fetchRoutines = useCallback(async () => {
    if (!workspaceId) return;
    const res = await listRoutines(workspaceId);
    setRoutines(res.routines ?? []);
  }, [workspaceId, setRoutines]);

  const fetchRuns = useCallback(async () => {
    if (!workspaceId) return [] as RoutineRun[];
    const res = await listAllRoutineRuns(workspaceId);
    return res.runs ?? [];
  }, [workspaceId]);

  const fetchTriggers = useCallback(async (rs: Routine[]) => {
    const entries = await Promise.all(
      rs.map(async (r) => {
        const res = await listRoutineTriggers(r.id).catch(() => ({
          triggers: [] as RoutineTrigger[],
        }));
        return [r.id, res.triggers ?? []] as const;
      }),
    );
    const out: Record<string, RoutineTrigger[]> = {};
    for (const [id, triggers] of entries) out[id] = triggers;
    return out;
  }, []);

  useEffect(() => {
    let cancelled = false;
    void fetchRoutines();
    fetchRuns().then((next) => {
      if (!cancelled) setRuns(next);
    });
    return () => {
      cancelled = true;
    };
  }, [fetchRoutines, fetchRuns]);

  useEffect(() => {
    let cancelled = false;
    if (routines.length === 0) {
      Promise.resolve().then(() => {
        if (!cancelled) setTriggersByRoutine({});
      });
      return () => {
        cancelled = true;
      };
    }
    fetchTriggers(routines).then((map) => {
      if (!cancelled) setTriggersByRoutine(map);
    });
    return () => {
      cancelled = true;
    };
  }, [routines, fetchTriggers]);

  return { routines, runs, setRuns, triggersByRoutine, fetchRoutines, fetchRuns };
}

export function RoutinesContent() {
  const { t } = useTranslation();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const agents = useAppStore(selectOfficeAgentProfiles);
  const [showCreate, setShowCreate] = useState(false);
  const { routines, runs, setRuns, triggersByRoutine, fetchRoutines, fetchRuns } =
    useRoutinesData(workspaceId);

  const { handleToggle, handleDelete, handleCreate } = useRoutineActions(
    workspaceId,
    fetchRoutines,
  );

  const handleRunNow = useCallback(
    async (id: string) => {
      try {
        await runRoutine(id);
        setRuns(await fetchRuns());
        toast.success(t("office:routineStarted"));
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t("office:failedToRunRoutine"));
      }
    },
    [fetchRuns, setRuns],
  );

  return (
    <div className="space-y-4 p-6">
      <div className="flex justify-end">
        <Button size="sm" onClick={() => setShowCreate(true)} className="cursor-pointer">
          <IconPlus className="h-4 w-4 mr-1" /> {t("office:newRoutine")}
        </Button>
      </div>

      <Tabs defaultValue="routines">
        <TabsList>
          <TabsTrigger value="routines" className="cursor-pointer">
            {t("office:all")}
          </TabsTrigger>
          <TabsTrigger value="runs" className="cursor-pointer">
            {t("office:runs")}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="routines">
          <RoutinesList
            routines={routines}
            agents={agents}
            triggersByRoutine={triggersByRoutine}
            onToggle={handleToggle}
            onRunNow={handleRunNow}
            onDelete={handleDelete}
          />
        </TabsContent>

        <TabsContent value="runs">
          <RunsList runs={runs} />
        </TabsContent>
      </Tabs>

      <CreateRoutineDialog
        open={showCreate}
        onOpenChange={setShowCreate}
        agents={agents}
        onSubmit={(data) => handleCreate(data, () => setShowCreate(false))}
      />
    </div>
  );
}

function RoutinesList({
  routines,
  agents,
  triggersByRoutine,
  onToggle,
  onRunNow,
  onDelete,
}: {
  routines: Routine[];
  agents: AgentProfile[];
  triggersByRoutine: Record<string, RoutineTrigger[]>;
  onToggle: (id: string, active: boolean) => void;
  onRunNow: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  const { t } = useTranslation();
  const [expandedId, setExpandedId] = useState<string | null>(null);

  if (routines.length === 0) {
    return (
      <EmptyState
        message={t("office:noRoutinesYet")}
        description={t("office:routinesAutomaticallyCreateTasksOnA")}
      />
    );
  }
  return (
    <div className="border border-border rounded-lg divide-y divide-border">
      {routines.map((routine) => (
        <RoutineRow
          key={routine.id}
          routine={routine}
          agents={agents}
          triggers={triggersByRoutine[routine.id] ?? []}
          expanded={expandedId === routine.id}
          onToggle={onToggle}
          onRunNow={onRunNow}
          onDelete={onDelete}
          onClick={(id) => setExpandedId(expandedId === id ? null : id)}
        />
      ))}
    </div>
  );
}

function RunsList({ runs }: { runs: RoutineRun[] }) {
  const { t } = useTranslation();
  if (runs.length === 0) {
    return (
      <EmptyState
        message={t("office:noRunsYet")}
        description={t("office:runsAppearHereWhenARoutine")}
      />
    );
  }
  return (
    <div className="border border-border rounded-lg divide-y divide-border">
      {runs.map((run) => (
        <RunRow key={run.id} run={run} />
      ))}
    </div>
  );
}
