"use client";

import { useCallback, useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { IconPlus } from "@tabler/icons-react";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import {
  listRoutines,
  createRoutine,
  updateRoutine,
  deleteRoutine,
  runRoutine,
  listAllRoutineRuns,
  createRoutineTrigger,
} from "@/lib/api/domains/orchestrate-api";
import type { Routine, AgentInstance, RoutineRun } from "@/lib/state/slices/orchestrate/types";
import { RoutineRow } from "./routine-row";
import { RunRow } from "./run-row";
import { CreateRoutineDialog } from "./create-routine-dialog";
import { EmptyState } from "../components/shared/empty-state";
import { PageHeader } from "../components/shared/page-header";

export function RoutinesContent() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const routines = useAppStore((s) => s.orchestrate.routines);
  const agents = useAppStore((s) => s.orchestrate.agentInstances);
  const setRoutines = useAppStore((s) => s.setRoutines);

  const [runs, setRuns] = useState<RoutineRun[]>([]);
  const [showCreate, setShowCreate] = useState(false);

  const fetchRoutines = useCallback(async () => {
    if (!workspaceId) return;
    const res = await listRoutines(workspaceId);
    setRoutines(res.routines ?? []);
  }, [workspaceId, setRoutines]);

  const fetchRuns = useCallback(async () => {
    if (!workspaceId) return;
    const res = await listAllRoutineRuns(workspaceId);
    setRuns(res.runs ?? []);
  }, [workspaceId]);

  useEffect(() => {
    fetchRoutines();
    fetchRuns();
  }, [fetchRoutines, fetchRuns]);

  const handleToggle = useCallback(
    async (id: string, active: boolean) => {
      try {
        await updateRoutine(id, { status: active ? "active" : "paused" } as Record<string, unknown>);
        await fetchRoutines();
        toast.success(active ? "Routine activated" : "Routine paused");
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to update routine");
      }
    },
    [fetchRoutines],
  );

  const handleRunNow = useCallback(
    async (id: string) => {
      try {
        await runRoutine(id);
        await fetchRuns();
        toast.success("Routine started");
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to run routine");
      }
    },
    [fetchRuns],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      try {
        await deleteRoutine(id);
        await fetchRoutines();
        toast.success("Routine deleted");
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to delete routine");
      }
    },
    [fetchRoutines],
  );

  const handleCreate = useCallback(
    async (data: {
      name: string;
      description: string;
      taskTitle: string;
      taskDescription: string;
      assigneeAgentInstanceId: string;
      concurrencyPolicy: string;
      triggerKind: string;
      cronExpression: string;
      timezone: string;
    }) => {
      if (!workspaceId) return;
      try {
        const template = JSON.stringify({ title: data.taskTitle, description: data.taskDescription });
        const res = await createRoutine(workspaceId, {
          name: data.name,
          description: data.description,
          taskTemplate: JSON.parse(template),
          assigneeAgentInstanceId: data.assigneeAgentInstanceId,
          concurrencyPolicy: data.concurrencyPolicy,
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
        setShowCreate(false);
        await fetchRoutines();
        toast.success("Routine created");
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to create routine");
      }
    },
    [workspaceId, fetchRoutines],
  );

  return (
    <div className="space-y-4 p-6">
      <PageHeader
        title="Routines"
        action={
          <Button size="sm" onClick={() => setShowCreate(true)} className="cursor-pointer">
            <IconPlus className="h-4 w-4 mr-1" /> New Routine
          </Button>
        }
      />

      <Tabs defaultValue="routines">
        <TabsList>
          <TabsTrigger value="routines" className="cursor-pointer">Routines</TabsTrigger>
          <TabsTrigger value="runs" className="cursor-pointer">Runs</TabsTrigger>
        </TabsList>

        <TabsContent value="routines">
          <RoutinesList
            routines={routines}
            agents={agents}
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
        onSubmit={handleCreate}
      />
    </div>
  );
}

function RoutinesList({
  routines,
  agents,
  onToggle,
  onRunNow,
  onDelete,
}: {
  routines: Routine[];
  agents: AgentInstance[];
  onToggle: (id: string, active: boolean) => void;
  onRunNow: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  if (routines.length === 0) {
    return (
      <EmptyState
        message="No routines yet."
        description="Create one to automate recurring tasks."
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
          onToggle={onToggle}
          onRunNow={onRunNow}
          onDelete={onDelete}
          onClick={() => {}}
        />
      ))}
    </div>
  );
}

function RunsList({ runs }: { runs: RoutineRun[] }) {
  if (runs.length === 0) {
    return <EmptyState message="No runs yet." />;
  }
  return (
    <div className="border border-border rounded-lg divide-y divide-border">
      {runs.map((run) => (
        <RunRow key={run.id} run={run} />
      ))}
    </div>
  );
}
