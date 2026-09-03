"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { listTasksByWorkspace } from "@/lib/api/domains/kanban-api";
import {
  getTaskDependencies,
  replaceTaskDependencies,
  type TaskDependencyProjectionResponse,
} from "@/lib/api/domains/task-dependencies-api";
import type { TaskDependencyCandidate } from "@/lib/types/task-dependencies";

export type TaskDependencyUpdateFailure = {
  readonly dependencyUpdate: true;
  readonly cause: unknown;
};

export function isTaskDependencyUpdateFailure(
  error: unknown,
): error is TaskDependencyUpdateFailure {
  return (
    typeof error === "object" &&
    error !== null &&
    "dependencyUpdate" in error &&
    (error as { dependencyUpdate?: unknown }).dependencyUpdate === true
  );
}

export type TaskEditDialogDependenciesState = {
  taskId: string | null;
  confirmedIds: string[];
  draftIds: string[];
  setDraftIds: (ids: string[]) => void;
  selectedTitles: Record<string, string>;
  candidates: TaskDependencyCandidate[];
  candidatesLoading: boolean;
  query: string;
  setQuery: (query: string) => void;
  loading: boolean;
  loadError: unknown | null;
  candidateError: unknown | null;
  saveError: unknown | null;
  error: unknown | null;
  submitError: TaskDependencyUpdateFailure | null;
  ready: boolean;
  isDirty: boolean;
  save: () => Promise<void>;
  retry: () => void;
  retryCandidates: () => void;
};

type TaskEditDialogDependenciesArgs = {
  open: boolean;
  workspaceId: string | null | undefined;
  taskId: string | null | undefined;
};

type DependencyRef = NonNullable<TaskDependencyProjectionResponse["depends_on"]>[number];

type DependencyProjectionState = {
  confirmedIds: string[];
  setConfirmedIds: (ids: string[]) => void;
  draftIds: string[];
  setDraftIds: (ids: string[]) => void;
  knownTitles: Record<string, string>;
  loading: boolean;
  loadError: unknown | null;
};

type DependencyCandidateState = {
  candidates: TaskDependencyCandidate[];
  candidatesLoading: boolean;
  candidateError: unknown | null;
};

type DependencySaveRequest = {
  identity: string;
  generation: number;
};

function uniqueIDs(ids: string[]): string[] {
  return [...new Set(ids)];
}

function sameIDs(left: string[], right: string[]): boolean {
  return left.length === right.length && left.every((id) => right.includes(id));
}

function dependencyRefs(response: TaskDependencyProjectionResponse): DependencyRef[] {
  return response.depends_on ?? [];
}

function mapCandidates(
  tasks: Array<{ id: string; title: string; archived_at?: string | null }>,
  taskId: string,
): TaskDependencyCandidate[] {
  return tasks
    .filter((task) => task.id !== taskId && task.archived_at == null)
    .map((task) => ({ id: task.id, title: task.title, isArchived: false }))
    .sort((left, right) => left.title.localeCompare(right.title));
}

function titlesFromRefs(refs: DependencyRef[]): Record<string, string> {
  return Object.fromEntries(
    refs
      .filter((ref): ref is DependencyRef & { title: string } => typeof ref.title === "string")
      .map((ref) => [ref.id, ref.title]),
  );
}

function useDependencySaveGuard(identity: string) {
  const identityRef = useRef(identity);
  const generationRef = useRef(0);
  if (identityRef.current !== identity) {
    identityRef.current = identity;
    generationRef.current += 1;
  }

  const begin = useCallback((): DependencySaveRequest => {
    const generation = generationRef.current + 1;
    generationRef.current = generation;
    return { identity: identityRef.current, generation };
  }, []);
  const isCurrent = useCallback((request: DependencySaveRequest) => {
    return identityRef.current === request.identity && generationRef.current === request.generation;
  }, []);

  return { begin, isCurrent };
}

function useTaskDependencyProjection(
  { open, workspaceId, taskId }: TaskEditDialogDependenciesArgs,
  reloadToken: number,
): DependencyProjectionState {
  const [confirmedIds, setConfirmedIds] = useState<string[]>([]);
  const [draftIds, setDraftIds] = useState<string[]>([]);
  const [knownTitles, setKnownTitles] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState<unknown | null>(null);

  useEffect(() => {
    let cancelled = false;
    if (!open || !workspaceId || !taskId) {
      setConfirmedIds([]);
      setDraftIds([]);
      setKnownTitles({});
      setLoading(false);
      setLoadError(null);
      return () => {
        cancelled = true;
      };
    }

    setLoading(true);
    setLoadError(null);
    void getTaskDependencies(taskId)
      .then((response) => {
        if (cancelled) return;
        const refs = dependencyRefs(response);
        const ids = refs.map((ref) => ref.id);
        setConfirmedIds(ids);
        setDraftIds(ids);
        setKnownTitles(titlesFromRefs(refs));
      })
      .catch((error: unknown) => {
        if (!cancelled) setLoadError(error);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [open, reloadToken, taskId, workspaceId]);

  return {
    confirmedIds,
    setConfirmedIds,
    draftIds,
    setDraftIds,
    knownTitles,
    loading,
    loadError,
  };
}

function useTaskDependencyCandidates(
  { open, workspaceId, taskId, query }: TaskEditDialogDependenciesArgs & { query: string },
  reloadToken: number,
): DependencyCandidateState {
  const [candidates, setCandidates] = useState<TaskDependencyCandidate[]>([]);
  const [candidatesLoading, setCandidatesLoading] = useState(false);
  const [candidateError, setCandidateError] = useState<unknown | null>(null);

  useEffect(() => {
    let cancelled = false;
    if (!open || !workspaceId || !taskId) {
      setCandidates([]);
      setCandidatesLoading(false);
      setCandidateError(null);
      return () => {
        cancelled = true;
      };
    }

    setCandidatesLoading(true);
    setCandidateError(null);
    void listAllDependencyCandidates(workspaceId, query)
      .then((tasks) => {
        if (!cancelled) setCandidates(mapCandidates(tasks, taskId));
      })
      .catch((error: unknown) => {
        if (!cancelled) setCandidateError(error);
      })
      .finally(() => {
        if (!cancelled) setCandidatesLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [open, query, reloadToken, taskId, workspaceId]);

  return { candidates, candidatesLoading, candidateError };
}

const dependencyCandidatePageSize = 100;

async function listAllDependencyCandidates(
  workspaceId: string,
  query: string,
): Promise<Array<{ id: string; title: string; archived_at?: string | null }>> {
  const tasks: Array<{ id: string; title: string; archived_at?: string | null }> = [];
  let page = 1;
  let total = 0;
  do {
    const response = await listTasksByWorkspace(workspaceId, {
      page,
      pageSize: dependencyCandidatePageSize,
      query,
    });
    tasks.push(...response.tasks);
    total = response.total;
    if (response.tasks.length === 0) break;
    page += 1;
  } while (tasks.length < total);
  return tasks;
}

export function useTaskEditDialogDependencies({
  open,
  workspaceId,
  taskId,
}: TaskEditDialogDependenciesArgs): TaskEditDialogDependenciesState {
  const [saveError, setSaveError] = useState<unknown | null>(null);
  const [submitError, setSubmitError] = useState<TaskDependencyUpdateFailure | null>(null);
  const [projectionReloadToken, setProjectionReloadToken] = useState(0);
  const [candidateReloadToken, setCandidateReloadToken] = useState(0);
  const [query, setQueryState] = useState("");
  const projection = useTaskDependencyProjection(
    { open, workspaceId, taskId },
    projectionReloadToken,
  );
  const candidateState = useTaskDependencyCandidates(
    { open, workspaceId, taskId, query },
    candidateReloadToken,
  );

  const saveIdentity = JSON.stringify([
    open,
    workspaceId ?? null,
    taskId ?? null,
    projectionReloadToken,
  ]);
  const saveGuard = useDependencySaveGuard(saveIdentity);

  useEffect(() => {
    setSaveError(null);
    setSubmitError(null);
    if (open && workspaceId && taskId) setQueryState("");
  }, [open, taskId, workspaceId]);

  const setDraftIds = useCallback(
    (ids: string[]) => {
      projection.setDraftIds(uniqueIDs(ids));
      setSaveError(null);
      setSubmitError(null);
    },
    [projection],
  );

  const setQuery = useCallback((value: string) => {
    setQueryState(value);
  }, []);

  const { confirmedIds, setConfirmedIds, draftIds, knownTitles, loading, loadError } = projection;
  const { candidates, candidatesLoading, candidateError } = candidateState;
  const isDirty = !sameIDs(confirmedIds, draftIds);

  const save = useCallback(async () => {
    if (!open || !workspaceId || !taskId || loading || !isDirty) return;
    const request = saveGuard.begin();
    const ids = [...draftIds];
    setSaveError(null);
    setSubmitError(null);
    try {
      await replaceTaskDependencies(taskId, ids);
      if (!saveGuard.isCurrent(request)) return;
      setConfirmedIds(ids);
    } catch (cause: unknown) {
      if (!saveGuard.isCurrent(request)) return;
      const failure: TaskDependencyUpdateFailure = { dependencyUpdate: true, cause };
      setSaveError(cause);
      setSubmitError(failure);
      throw failure;
    }
  }, [draftIds, isDirty, loading, open, saveGuard, taskId, workspaceId]);

  const selectedTitles = useMemo(
    () => ({
      ...knownTitles,
      ...Object.fromEntries(candidates.map((candidate) => [candidate.id, candidate.title])),
    }),
    [candidates, knownTitles],
  );
  const retry = useCallback(() => setProjectionReloadToken((value) => value + 1), []);
  const retryCandidates = useCallback(() => setCandidateReloadToken((value) => value + 1), []);
  const ready = !loading && !loadError;

  return {
    taskId: taskId ?? null,
    confirmedIds,
    draftIds,
    setDraftIds,
    selectedTitles,
    candidates,
    candidatesLoading,
    query,
    setQuery,
    loading,
    loadError,
    candidateError,
    saveError,
    error: saveError ?? loadError ?? candidateError,
    submitError,
    ready,
    isDirty,
    save,
    retry,
    retryCandidates,
  };
}
