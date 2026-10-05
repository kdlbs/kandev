import type { FormEvent } from "react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "@/lib/api/client";
import { sendMessageRequest } from "@/hooks/message-request";
import { useAgentProjectMutations } from "@/hooks/domains/agent-projects/use-agent-projects";
import type { RemoteRepository } from "@/hooks/domains/integrations/use-remote-repositories";
import { ensureTaskSession } from "@/lib/services/session-launch-service";
import { generateUUID } from "@/lib/utils";
import {
  PENDING_PROJECT_REPOSITORY_PREFIX,
  projectRemoteRepositoryKey,
  projectRemoteSelectionId,
  projectRepositoryMatchesRemote,
  resolveAgentProjectRepositories,
  ProjectRepositoryResolutionError,
} from "@/lib/agent-projects/repository-resolution";
import type { AgentProject } from "@/lib/types/http-agent-projects";
import { draftFor, useProjectFormData } from "./agent-project-form-fields";
import type { ProjectDraft } from "./agent-project-form-fields";

function pendingRemoteSelectionId(repository: RemoteRepository) {
  return projectRemoteSelectionId(repository);
}

function pendingRemoteForSelection(selectionId: string, draft: ProjectDraft) {
  if (!selectionId.startsWith(PENDING_PROJECT_REPOSITORY_PREFIX)) return undefined;
  const key = selectionId.slice(PENDING_PROJECT_REPOSITORY_PREFIX.length);
  return draft.remoteRepositories.find(
    (repository) => projectRemoteRepositoryKey(repository) === key,
  );
}

function effectiveEconomyProfileId(draft: ProjectDraft) {
  return draft.economyProfileInherited ? draft.coordinatorProfileId : draft.economyProfileId;
}

function effectiveFrontierProfileId(draft: ProjectDraft) {
  return draft.frontierProfileInherited ? draft.coordinatorProfileId : draft.frontierProfileId;
}

function isProjectFormReady(
  draft: ProjectDraft,
  project: AgentProject | undefined,
  formData: ReturnType<typeof useProjectFormData>,
) {
  const profileIds = [
    draft.coordinatorProfileId,
    effectiveEconomyProfileId(draft),
    effectiveFrontierProfileId(draft),
  ];
  const repositoryIdsAvailable = draft.repositoryIds.every(
    (id) =>
      Boolean(pendingRemoteForSelection(id, draft)) ||
      formData.repositories.some((repository) => repository.id === id),
  );
  const profileIdsAvailable = profileIds.every((id) =>
    formData.profiles.some((profile) => profile.id === id),
  );
  return (
    draft.name.trim().length > 0 &&
    draft.repositoryIds.length > 0 &&
    draft.repositoryIds.includes(draft.primaryRepositoryId) &&
    profileIds.every(Boolean) &&
    (Boolean(project) || formData.executorReady) &&
    formData.repositoriesLoaded &&
    repositoryIdsAvailable &&
    profileIdsAvailable
  );
}

async function persistProjectDraft({
  mutations,
  workspaceId,
  project,
  draft,
  applyDefaultExecutor,
  requestKey,
}: {
  mutations: ReturnType<typeof useAgentProjectMutations>;
  workspaceId: string;
  project: AgentProject | undefined;
  draft: ProjectDraft;
  applyDefaultExecutor: boolean;
  requestKey: string;
}): Promise<AgentProject | undefined> {
  const payload = {
    name: draft.name.trim(),
    repositoryIds: draft.repositoryIds,
    primaryRepositoryId: draft.primaryRepositoryId,
    coordinatorProfileId: draft.coordinatorProfileId,
    economyProfileId: effectiveEconomyProfileId(draft),
    frontierProfileId: effectiveFrontierProfileId(draft),
  };
  if (project) {
    await mutations.update(workspaceId, project.id, {
      ...payload,
      revision: project.revision,
      applyWorkspaceDefaultExecutor: applyDefaultExecutor || undefined,
    });
    return undefined;
  }
  return mutations.create(workspaceId, { ...payload, requestKey });
}

type PendingProjectStart = {
  generation: number;
  workspaceId: string;
  requestKey: string;
  draft: ProjectDraft;
  project?: AgentProject;
  sessionId?: string;
  messageId: string;
  messagePayload?: Parameters<typeof sendMessageRequest>[0];
};

function getErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

type ControllerScope = {
  open: boolean;
  workspaceId: string;
  project: AgentProject | undefined;
};

type AgentProjectFormControllerOptions = ControllerScope & {
  formData: ReturnType<typeof useProjectFormData>;
  onOpenChange: (open: boolean) => void;
  onOpenCoordinator?: (taskId: string) => void;
};

function useControllerState({ open, workspaceId, project }: ControllerScope) {
  const [draft, setDraft] = useState<ProjectDraft>(() => draftFor(project));
  const [applyDefaultExecutor, setApplyDefaultExecutor] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [createdProject, setCreatedProject] = useState<AgentProject | undefined>();
  const createRequestKey = useRef<string | null>(null);
  const createRetryDraft = useRef<ProjectDraft | null>(null);
  const pendingStart = useRef<PendingProjectStart | null>(null);
  const repositoryResolutionCache = useRef(
    new Map<string, Awaited<ReturnType<typeof resolveAgentProjectRepositories>>[number]>(),
  );
  const generation = useRef(0);
  const inFlightGeneration = useRef<number | null>(null);

  useEffect(() => {
    generation.current += 1;
    const currentGeneration = generation.current;
    if (!open) {
      createRequestKey.current = null;
      createRetryDraft.current = null;
      pendingStart.current = null;
      setCreatedProject(undefined);
      setSaving(false);
      return () => {
        if (generation.current === currentGeneration) generation.current += 1;
      };
    }
    createRequestKey.current = project ? null : (createRequestKey.current ?? generateUUID());
    createRetryDraft.current = null;
    pendingStart.current = null;
    repositoryResolutionCache.current.clear();
    setDraft(draftFor(project));
    setApplyDefaultExecutor(false);
    setCreatedProject(undefined);
    setError(null);
    setSaving(false);
    return () => {
      if (generation.current === currentGeneration) generation.current += 1;
      if (inFlightGeneration.current === currentGeneration) inFlightGeneration.current = null;
    };
  }, [open, project, workspaceId]);

  return {
    open,
    workspaceId,
    project,
    draft,
    setDraft,
    applyDefaultExecutor,
    setApplyDefaultExecutor,
    error,
    setError,
    saving,
    setSaving,
    createdProject,
    setCreatedProject,
    createRequestKey,
    createRetryDraft,
    pendingStart,
    repositoryResolutionCache,
    generation,
    inFlightGeneration,
  };
}

type ControllerRuntime = ReturnType<typeof useControllerState> &
  Pick<AgentProjectFormControllerOptions, "formData" | "onOpenChange" | "onOpenCoordinator"> & {
    t: ReturnType<typeof useTranslation>["t"];
    mutations: ReturnType<typeof useAgentProjectMutations>;
  };

function isCurrent(runtime: ControllerRuntime, token: number) {
  return runtime.open && runtime.generation.current === token;
}

function resolveDraftRepositories(
  snapshot: ProjectDraft,
  runtime: ControllerRuntime,
): ProjectDraft | Promise<ProjectDraft> {
  const pendingIds = snapshot.repositoryIds.filter((id) =>
    id.startsWith(PENDING_PROJECT_REPOSITORY_PREFIX),
  );
  if (pendingIds.length === 0) return snapshot;
  const selected = snapshot.remoteRepositories.filter((repository) =>
    pendingIds.includes(pendingRemoteSelectionId(repository)),
  );
  if (selected.length !== pendingIds.length || !runtime.formData.createRemoteRepository) {
    throw new Error(runtime.t("projects:repositoryResolutionUnavailable"));
  }
  return resolveAgentProjectRepositories({
    workspaceId: runtime.workspaceId,
    selected,
    existing: runtime.formData.repositories,
    cache: runtime.repositoryResolutionCache.current,
    create: runtime.formData.createRemoteRepository,
  }).then((resolved) => mapResolvedRepositories(snapshot, pendingIds, selected, resolved));
}

function mapResolvedRepositories(
  snapshot: ProjectDraft,
  pendingIds: string[],
  selected: RemoteRepository[],
  resolved: Awaited<ReturnType<typeof resolveAgentProjectRepositories>>,
) {
  const resolvedBySelectionId = new Map(
    selected.map((repository, index) => [
      pendingRemoteSelectionId(repository),
      resolved[index]!.id,
    ]),
  );
  return {
    ...snapshot,
    repositoryIds: snapshot.repositoryIds.map((id) => resolvedBySelectionId.get(id) ?? id),
    primaryRepositoryId:
      resolvedBySelectionId.get(snapshot.primaryRepositoryId) ?? snapshot.primaryRepositoryId,
    remoteRepositories: snapshot.remoteRepositories.filter(
      (repository) => !pendingIds.includes(pendingRemoteSelectionId(repository)),
    ),
  };
}

function createDraftUpdater(
  setDraft: ControllerRuntime["setDraft"],
  promptLocked: boolean,
  createRetryDraft: ControllerRuntime["createRetryDraft"],
) {
  return (patch: Partial<ProjectDraft>) => {
    if (promptLocked && patch.initialPrompt !== undefined) return;
    if (createRetryDraft.current) return;
    const normalized = { ...patch };
    if (patch.economyProfileId !== undefined && patch.economyProfileInherited === undefined) {
      normalized.economyProfileInherited = false;
    }
    if (patch.frontierProfileId !== undefined && patch.frontierProfileInherited === undefined) {
      normalized.frontierProfileInherited = false;
    }
    setDraft((current) => ({ ...current, ...normalized }));
  };
}

function createRepositoryActions(
  runtime: ControllerRuntime,
  draft: ProjectDraft,
  updateDraft: (patch: Partial<ProjectDraft>) => void,
  promptLocked: boolean,
) {
  const canChangeRepositories = () => !promptLocked && !runtime.createRetryDraft.current;
  const toggleRepository = (repositoryId: string) => {
    if (!canChangeRepositories()) return;
    const currentlySelected = draft.repositoryIds.includes(repositoryId);
    const selectedRepository = runtime.formData.repositories.find(
      (repository) => repository.id === repositoryId,
    );
    const matchingRemoteSelections =
      !currentlySelected && selectedRepository
        ? draft.remoteRepositories.filter((repository) =>
            projectRepositoryMatchesRemote(selectedRepository, repository),
          )
        : [];
    const replacedRemoteIds = new Set(matchingRemoteSelections.map(pendingRemoteSelectionId));
    const repositoryIds = currentlySelected
      ? draft.repositoryIds.filter((id) => id !== repositoryId)
      : [...draft.repositoryIds.filter((id) => !replacedRemoteIds.has(id)), repositoryId];
    const remoteRepositories = nextRemoteRepositories(
      draft.remoteRepositories,
      repositoryId,
      replacedRemoteIds,
      currentlySelected,
    );
    const replacedPrimary = replacedRemoteIds.has(draft.primaryRepositoryId);
    let primaryRepositoryId = repositoryIds[0] ?? "";
    if (repositoryIds.includes(draft.primaryRepositoryId)) {
      primaryRepositoryId = draft.primaryRepositoryId;
    }
    if (replacedPrimary) primaryRepositoryId = repositoryId;
    updateDraft({ repositoryIds, remoteRepositories, primaryRepositoryId });
  };
  const selectRemoteRepository = (repository: RemoteRepository) => {
    if (!canChangeRepositories()) return;
    const matching = runtime.formData.repositories.filter(
      (candidate) =>
        draft.repositoryIds.includes(candidate.id) &&
        projectRepositoryMatchesRemote(candidate, repository),
    );
    if (matching.length > 1) {
      runtime.setError(runtime.t("projects:repositoryAmbiguous"));
      return;
    }
    if (matching.length === 1) return;
    const selectionId = pendingRemoteSelectionId(repository);
    if (draft.repositoryIds.includes(selectionId)) return;
    updateDraft({
      repositoryIds: [...draft.repositoryIds, selectionId],
      remoteRepositories: [...draft.remoteRepositories, repository],
      primaryRepositoryId: draft.primaryRepositoryId || selectionId,
    });
  };
  return { toggleRepository, selectRemoteRepository };
}

function nextRemoteRepositories(
  repositories: RemoteRepository[],
  repositoryId: string,
  replacedRemoteIds: Set<string>,
  currentlySelected: boolean,
) {
  if (!currentlySelected && replacedRemoteIds.size === 0) return repositories;
  return repositories.filter(
    (repository) =>
      pendingRemoteSelectionId(repository) !== repositoryId &&
      !replacedRemoteIds.has(pendingRemoteSelectionId(repository)),
  );
}

function closeProjectDialog(runtime: ControllerRuntime) {
  runtime.generation.current += 1;
  runtime.inFlightGeneration.current = null;
  runtime.onOpenChange(false);
}

function finishProjectStart(runtime: ControllerRuntime, pending: PendingProjectStart) {
  const taskId = pending.project?.main_task_id;
  if (taskId) runtime.onOpenCoordinator?.(taskId);
  closeProjectDialog(runtime);
}

async function sendPendingPrompt(
  runtime: ControllerRuntime,
  pending: PendingProjectStart,
  token: number,
) {
  const savedProject = pending.project;
  if (!savedProject?.main_task_id) throw new Error(runtime.t("projects:coordinatorUnavailable"));
  if (!pending.sessionId) {
    const ensured = await ensureTaskSession(savedProject.main_task_id, {
      ensureExecution: true,
      autoStart: false,
      activationSource: "user_action",
    });
    if (!isCurrent(runtime, token)) return false;
    if (!ensured.success || !ensured.session_id) {
      throw new Error(runtime.t("projects:coordinatorSessionUnavailable"));
    }
    pending.sessionId = ensured.session_id;
  }
  pending.messagePayload ??= {
    taskId: savedProject.main_task_id,
    resolvedSessionId: pending.sessionId,
    clientMessageId: pending.messageId,
    finalMessage: pending.draft.initialPrompt,
    modelToSend: undefined,
    planMode: false,
  };
  await sendMessageRequest(pending.messagePayload);
  return isCurrent(runtime, token);
}

async function runPendingStart(
  runtime: ControllerRuntime,
  pending: PendingProjectStart,
  token: number,
) {
  if ((await sendPendingPrompt(runtime, pending, token)) && isCurrent(runtime, token)) {
    finishProjectStart(runtime, pending);
  }
}

async function runProjectSubmission(
  runtime: ControllerRuntime,
  snapshot: ProjectDraft,
  token: number,
) {
  const requestKey = (runtime.createRequestKey.current ??= generateUUID());
  const resolution = resolveDraftRepositories(snapshot, runtime);
  const resolvedDraft = resolution instanceof Promise ? await resolution : resolution;
  if (!isCurrent(runtime, token)) return;
  runtime.setDraft(resolvedDraft);
  const pending = createPendingStart(snapshot, resolvedDraft, requestKey, runtime, token);
  if (pending) runtime.pendingStart.current = pending;
  if (!runtime.project) runtime.createRetryDraft.current = { ...resolvedDraft };
  const created = await persistProjectDraft({
    mutations: runtime.mutations,
    workspaceId: runtime.workspaceId,
    project: runtime.project,
    draft: resolvedDraft,
    applyDefaultExecutor: runtime.applyDefaultExecutor,
    requestKey,
  });
  if (!isCurrent(runtime, token)) return;
  runtime.createRetryDraft.current = null;
  if (runtime.project) {
    runtime.onOpenChange(false);
    return;
  }
  if (!created) throw new Error(runtime.t("projects:createFailed"));
  runtime.setCreatedProject(created);
  if (!pending) {
    runtime.pendingStart.current = null;
    runtime.onOpenChange(false);
    return;
  }
  pending.project = created;
  await runPendingStart(runtime, pending, token);
}

function createPendingStart(
  originalDraft: ProjectDraft,
  resolvedDraft: ProjectDraft,
  requestKey: string,
  runtime: ControllerRuntime,
  token: number,
): PendingProjectStart | null {
  if (!originalDraft.initialPrompt.trim()) return null;
  return {
    generation: token,
    workspaceId: runtime.workspaceId,
    requestKey,
    draft: { ...resolvedDraft },
    messageId: generateUUID(),
  } satisfies PendingProjectStart;
}

function userErrorMessage(caught: unknown, t: ControllerRuntime["t"]) {
  if (caught instanceof ProjectRepositoryResolutionError) {
    let key = "projects:repositoryRecordMismatch";
    if (caught.code === "ineligible") key = "projects:repositoryIneligible";
    else if (caught.code === "ambiguous") key = "projects:repositoryAmbiguous";
    return t(key);
  }
  if (
    caught instanceof ApiError &&
    caught.status === 422 &&
    caught.message.includes("agent profile does not support project workspace access")
  ) {
    return t("projects:unsupportedProfile");
  }
  return getErrorMessage(caught);
}

async function submitProjectDraft(
  runtime: ControllerRuntime,
  draft: ProjectDraft,
  ready: boolean,
  event: FormEvent,
) {
  event.preventDefault();
  if (!ready || runtime.inFlightGeneration.current !== null || runtime.createdProject) return;
  const token = runtime.generation.current;
  runtime.inFlightGeneration.current = token;
  runtime.setSaving(true);
  runtime.setError(null);
  try {
    await runProjectSubmission(runtime, runtime.createRetryDraft.current ?? { ...draft }, token);
  } catch (caught) {
    handleSubmissionError(runtime, caught, token);
  } finally {
    finishSubmission(runtime, token);
  }
}

function handleSubmissionError(runtime: ControllerRuntime, caught: unknown, token: number) {
  if (!isCurrent(runtime, token)) return;
  if (
    !runtime.project &&
    runtime.createRetryDraft.current &&
    caught instanceof ApiError &&
    (caught.status === 400 || caught.status === 422)
  ) {
    runtime.createRetryDraft.current = null;
  }
  runtime.setError(userErrorMessage(caught, runtime.t));
}

function finishSubmission(runtime: ControllerRuntime, token: number) {
  if (runtime.inFlightGeneration.current === token) runtime.inFlightGeneration.current = null;
  if (isCurrent(runtime, token)) runtime.setSaving(false);
}

async function retryProjectStart(runtime: ControllerRuntime) {
  const pending = runtime.pendingStart.current;
  if (!pending?.project || runtime.inFlightGeneration.current !== null) return;
  const token = runtime.generation.current;
  runtime.inFlightGeneration.current = token;
  runtime.setSaving(true);
  runtime.setError(null);
  try {
    await runPendingStart(runtime, pending, token);
  } catch (caught) {
    if (isCurrent(runtime, token)) runtime.setError(userErrorMessage(caught, runtime.t));
  } finally {
    finishSubmission(runtime, token);
  }
}

export function useAgentProjectFormController(options: AgentProjectFormControllerOptions) {
  const { t } = useTranslation();
  const mutations = useAgentProjectMutations();
  const runtime: ControllerRuntime = {
    ...useControllerState(options),
    ...options,
    t,
    mutations,
  };
  const { draft } = runtime;
  const ready = useMemo(
    () => isProjectFormReady(draft, runtime.project, runtime.formData),
    [draft, runtime.formData, runtime.project],
  );
  const promptLocked = Boolean(runtime.pendingStart.current?.project);
  const updateDraft = createDraftUpdater(runtime.setDraft, promptLocked, runtime.createRetryDraft);
  const repositoryActions = createRepositoryActions(runtime, draft, updateDraft, promptLocked);
  const closeDialog = () => closeProjectDialog(runtime);
  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      runtime.generation.current += 1;
      runtime.inFlightGeneration.current = null;
    }
    runtime.onOpenChange(nextOpen);
  };
  return {
    draft,
    updateDraft,
    ...repositoryActions,
    applyDefaultExecutor: runtime.applyDefaultExecutor,
    setApplyDefaultExecutor: runtime.setApplyDefaultExecutor,
    ready,
    error: runtime.error,
    saving: runtime.saving,
    createdProject: runtime.createdProject,
    promptLocked,
    createRetryLocked: Boolean(runtime.createRetryDraft.current),
    effectiveEconomyProfileId: effectiveEconomyProfileId(draft),
    effectiveFrontierProfileId: effectiveFrontierProfileId(draft),
    submit: (event: FormEvent) => submitProjectDraft(runtime, draft, ready, event),
    retryStart: () => retryProjectStart(runtime),
    openCoordinator: () => {
      const taskId = runtime.createdProject?.main_task_id;
      if (taskId) runtime.onOpenCoordinator?.(taskId);
      closeDialog();
    },
    closeDialog,
    handleOpenChange,
  };
}
