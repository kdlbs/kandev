import { useCallback, useEffect, useRef, useState } from "react";
import {
  fetchTask,
  fetchWorkflowSnapshot,
  listAgents,
  listExecutors,
  listWorkflows,
} from "@/lib/api";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { Task, Workflow, WorkflowSnapshot } from "@/lib/types/http";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";

export type ChangeWorkflowLoadStatus = "idle" | "loading" | "success" | "error";
type OpenTaskArgs = { open: boolean; taskId: string | null; workspaceId: string | null };

export function useChangeWorkflowTask({ open, taskId, workspaceId }: OpenTaskArgs) {
  const [task, setTask] = useState<Task | null>(null);
  const [status, setStatus] = useState<ChangeWorkflowLoadStatus>("idle");
  const [error, setError] = useState<unknown>(null);
  const generationRef = useRef(0);
  const refreshGenerationRef = useRef(0);
  const taskIdRef = useRef(taskId);
  const workspaceIdRef = useRef(workspaceId);
  taskIdRef.current = taskId;
  workspaceIdRef.current = workspaceId;

  useEffect(() => {
    if (!open || !taskId || !workspaceId) return;
    const generation = ++generationRef.current;
    setTask(null);
    setStatus("loading");
    setError(null);
    void fetchTask(taskId, { cache: "no-store" }).then(
      (result) => {
        if (generationRef.current !== generation) return;
        if (result.workspace_id !== workspaceId) {
          setError(new Error());
          setStatus("error");
          return;
        }
        setTask(result);
        setStatus("success");
      },
      (nextError: unknown) => {
        if (generationRef.current !== generation) return;
        setError(nextError);
        setStatus("error");
      },
    );
    return () => {
      generationRef.current += 1;
    };
  }, [open, taskId, workspaceId]);

  const refreshTask = useCallback(async () => {
    const currentTaskId = taskIdRef.current;
    const currentWorkspaceId = workspaceIdRef.current;
    if (!currentTaskId || !currentWorkspaceId) return null;
    const generation = generationRef.current;
    const refreshGeneration = ++refreshGenerationRef.current;
    setStatus("loading");
    setError(null);
    try {
      const latest = await fetchTask(currentTaskId, { cache: "no-store" });
      if (
        generationRef.current !== generation ||
        refreshGenerationRef.current !== refreshGeneration ||
        taskIdRef.current !== currentTaskId ||
        workspaceIdRef.current !== currentWorkspaceId
      )
        return null;
      if (latest.workspace_id !== currentWorkspaceId) throw new Error();
      setTask(latest);
      setStatus("success");
      return latest;
    } catch (nextError) {
      if (
        generationRef.current === generation &&
        refreshGenerationRef.current === refreshGeneration &&
        taskIdRef.current === currentTaskId &&
        workspaceIdRef.current === currentWorkspaceId
      ) {
        setError(nextError);
        setStatus("error");
      }
      return null;
    }
  }, []);

  return { task, status, error, setTask, setStatus, setError, refreshTask };
}

export function useChangeWorkflowCatalog(open: boolean, workspaceId: string | null) {
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [status, setStatus] = useState<ChangeWorkflowLoadStatus>("idle");
  const [error, setError] = useState<unknown>(null);
  const [loadVersion, setLoadVersion] = useState(0);

  useEffect(() => {
    if (!open || !workspaceId) return;
    let active = true;
    setStatus("loading");
    setError(null);
    void listWorkflows(workspaceId, { cache: "no-store" }).then(
      (result) => {
        if (!active) return;
        setWorkflows(
          result.workflows.filter(
            (workflow) => workflow.workspace_id === workspaceId && !workflow.hidden,
          ),
        );
        setStatus("success");
      },
      (nextError: unknown) => {
        if (!active) return;
        setError(nextError);
        setStatus("error");
      },
    );
    return () => {
      active = false;
    };
  }, [open, workspaceId, loadVersion]);

  const retry = useCallback(() => setLoadVersion((version) => version + 1), []);
  return { workflows, status, error, retry };
}

export function useChangeWorkflowProfiles(open: boolean, workspaceId: string | null) {
  const [profiles, setProfiles] = useState<AgentProfileOption[]>([]);
  const [executors, setExecutors] = useState<
    Awaited<ReturnType<typeof listExecutors>>["executors"]
  >([]);
  const [status, setStatus] = useState<ChangeWorkflowLoadStatus>("idle");
  const [error, setError] = useState<unknown>(null);
  const [loadVersion, setLoadVersion] = useState(0);

  useEffect(() => {
    if (!open || !workspaceId) return;
    let active = true;
    setStatus("loading");
    setError(null);
    Promise.all([listAgents({ cache: "no-store" }), listExecutors({ cache: "no-store" })]).then(
      ([agents, executorResponse]) => {
        if (!active) return;
        setProfiles(
          agents.agents.flatMap((agent) =>
            agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
          ),
        );
        setExecutors(executorResponse.executors);
        setStatus("success");
      },
      (nextError: unknown) => {
        if (!active) return;
        setError(nextError);
        setStatus("error");
      },
    );
    return () => {
      active = false;
    };
  }, [open, workspaceId, loadVersion]);

  const retry = useCallback(() => setLoadVersion((version) => version + 1), []);
  return { profiles, executors, status, error, retry };
}

export function useChangeWorkflowDestination(open: boolean) {
  const [selectedWorkflowId, setSelectedWorkflowId] = useState("");
  const [selectedStepId, setSelectedStepId] = useState("");
  const [overrides, setOverrides] = useState<Record<string, string>>({});
  const [snapshot, setSnapshot] = useState<WorkflowSnapshot | null>(null);
  const [status, setStatus] = useState<ChangeWorkflowLoadStatus>("idle");
  const [error, setError] = useState<unknown>(null);
  const generationRef = useRef(0);

  useEffect(() => {
    if (!open) return;
    generationRef.current += 1;
    setSelectedWorkflowId("");
    setSelectedStepId("");
    setOverrides({});
    setSnapshot(null);
    setStatus("idle");
    setError(null);
  }, [open]);

  const loadSnapshot = useCallback((workflowId: string) => {
    const generation = ++generationRef.current;
    setSnapshot(null);
    setError(null);
    if (!workflowId) {
      setStatus("idle");
      return;
    }
    setStatus("loading");
    void fetchWorkflowSnapshot(workflowId, { cache: "no-store" }).then(
      (result) => {
        if (generationRef.current !== generation) return;
        setSnapshot(result);
        setStatus("success");
      },
      (nextError: unknown) => {
        if (generationRef.current !== generation) return;
        setError(nextError);
        setStatus("error");
      },
    );
  }, []);

  const changeWorkflow = useCallback(
    (workflowId: string) => {
      setSelectedWorkflowId(workflowId);
      setSelectedStepId("");
      setOverrides({});
      loadSnapshot(workflowId);
    },
    [loadSnapshot],
  );

  const setOverride = useCallback((sourceProfileId: string, replacementProfileId: string) => {
    setOverrides((current) => {
      const next = { ...current };
      if (!replacementProfileId || replacementProfileId === sourceProfileId)
        delete next[sourceProfileId];
      else next[sourceProfileId] = replacementProfileId;
      return next;
    });
  }, []);

  const retry = useCallback(
    () => loadSnapshot(selectedWorkflowId),
    [loadSnapshot, selectedWorkflowId],
  );
  const chooseStep = useCallback((stepId: string) => setSelectedStepId(stepId), []);
  return {
    selectedWorkflowId,
    selectedStepId,
    overrides,
    snapshot,
    status,
    error,
    changeWorkflow,
    chooseStep,
    setOverride,
    retry,
  };
}
