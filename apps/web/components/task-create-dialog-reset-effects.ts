import { useEffect } from "react";
import type { TaskCreateDialogInitialValues, TaskRemoteRepoRow } from "./task-create-dialog-types";
import { resetTaskForm, type FormResetters } from "./task-create-dialog-form-reset";
import { getTaskCreateDraft } from "@/lib/local-storage";
import { createDebugLogger } from "@/lib/debug/log";
import { clampTaskTitleInput, truncateRemoteTaskTitle } from "@/lib/task-title";

const stateDebug = createDebugLogger("task-create:state");

type FormResetEffectsArgs = {
  open: boolean;
  workspaceId: string | null;
  workflowId: string | null;
  initialValues: TaskCreateDialogInitialValues | undefined;
  resetters: FormResetters;
  setDraftDescription: (v: string) => void;
  setCurrentDefaults: (v: { name: string; description: string }) => void;
  setOpenCycle: React.Dispatch<React.SetStateAction<number>>;
  prevDialogRef: React.RefObject<{
    open: boolean;
    workspaceId: string | null;
    workflowId: string | null;
  }>;
  lockedWorkflow: boolean;
};

function isInitialLockedHydration(
  lockedWorkflow: boolean,
  previous: { workspaceId: string | null; workflowId: string | null },
): boolean {
  return lockedWorkflow && (previous.workspaceId === null || previous.workflowId === null);
}

export function useFormResetEffects({
  open,
  workspaceId,
  workflowId,
  initialValues,
  resetters,
  setDraftDescription,
  setCurrentDefaults,
  setOpenCycle,
  prevDialogRef,
  lockedWorkflow,
}: FormResetEffectsArgs) {
  useEffect(() => {
    const previous = prevDialogRef.current;
    (prevDialogRef as React.MutableRefObject<typeof previous>).current = {
      open,
      workspaceId,
      workflowId,
    };

    // Initial locked-context hydration preserves the user's in-progress draft.
    const initialLockedHydration = isInitialLockedHydration(lockedWorkflow, previous);
    if (
      !open ||
      (previous.open && (previous.workspaceId === workspaceId || initialLockedHydration))
    ) {
      return;
    }

    setOpenCycle((c) => c + 1);

    const defaults = resolveFormDefaults(initialValues, workspaceId);
    stateDebug("open-reset", {
      workspace_id: workspaceId ?? "-",
      workflow_id: workflowId ?? "-",
      source: defaults.source,
      title_present: defaults.name.trim().length > 0,
      description_present: defaults.description.trim().length > 0,
      initial_repository_id: initialValues?.repositoryId ?? "-",
      initial_branch: initialValues?.branch ?? initialValues?.checkoutBranch ?? "-",
    });
    setCurrentDefaults(defaults);
    resetTaskForm(
      resetters,
      defaults.name,
      defaults.description,
      lockedWorkflow ? workflowId : null,
      initialValues,
    );
    setDraftDescription(defaults.description);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lockedWorkflow, open, workflowId, workspaceId]);

  useEffect(() => {
    if (!open) return;
    stateDebug("discovery-reset", {
      workspace_id: workspaceId ?? "-",
      remote_url: initialValues?.remoteUrl ?? initialValues?.githubUrl ?? "-",
      seeded_remote_branch: initialValues?.checkoutBranch ?? initialValues?.branch ?? "-",
    });
    resetDiscoveryState(resetters, initialValues);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, workspaceId]);
}

/** Checks if initialValues has any user-provided content */
function hasUserContent(initialValues?: TaskCreateDialogInitialValues): boolean {
  const title = initialValues?.title ?? "";
  const description = initialValues?.description ?? "";
  return title.trim().length > 0 || description.trim().length > 0;
}

/** Resolves form defaults from draft (for create) or initialValues (for edit) */
function resolveFormDefaults(
  initialValues: TaskCreateDialogInitialValues | undefined,
  workspaceId: string | null,
) {
  const draft =
    !hasUserContent(initialValues) && workspaceId ? getTaskCreateDraft(workspaceId) : null;
  const initTitle = initialValues?.title ?? "";
  const initDesc = initialValues?.description ?? "";
  let name = initTitle;
  if (draft?.title) {
    name = clampTaskTitleInput(draft.title);
  } else if (initialValues?.githubUrl) {
    name = truncateRemoteTaskTitle(initTitle);
  }
  return {
    name,
    description: draft?.description ?? initDesc,
    source: resolveDefaultsSource(Boolean(draft), initialValues),
  };
}

function resolveDefaultsSource(
  hasDraft: boolean,
  initialValues: TaskCreateDialogInitialValues | undefined,
) {
  if (hasDraft) return "draft";
  if (hasUserContent(initialValues)) return "initial-values";
  return "empty";
}

function firstDefined<T>(...values: Array<T | undefined>): T | undefined {
  return values.find((value) => value !== undefined);
}

function definedOr<T>(fallback: T, ...values: Array<T | undefined>): T {
  return firstDefined(...values) ?? fallback;
}

function remoteRepositoryFullName(
  inspection: TaskCreateDialogInitialValues["remoteRepository"],
): string | undefined {
  return inspection ? `${inspection.ownerOrProject}/${inspection.repositoryName}` : undefined;
}

function seededRemoteRepositories(iv?: TaskCreateDialogInitialValues): TaskRemoteRepoRow[] {
  const initial: Partial<TaskCreateDialogInitialValues> = iv ?? {};
  const inspection = initial.remoteRepository;
  const repository: Partial<NonNullable<TaskCreateDialogInitialValues["remoteRepository"]>> =
    inspection ?? {};
  const remoteUrl = definedOr("", initial.remoteUrl, initial.githubUrl, repository.cloneUrl);
  if (!remoteUrl) return [];
  // Seed a pre-filled URL and preserve its PR head when one is provided.
  const seededBranch = definedOr(
    "",
    initial.checkoutBranch,
    initial.branch,
    repository.headBranch,
    repository.defaultBranch,
  );
  return [
    {
      key: "remote-0",
      url: remoteUrl,
      branch: seededBranch,
      source: "paste",
      prNumber: firstDefined(initial.prNumber, repository.pullRequest?.number),
      prBaseBranch: firstDefined(initial.prBaseBranch, repository.baseBranch),
      prHeadBranch: firstDefined(initial.checkoutBranch, repository.headBranch),
      remoteUrl: repository.cloneUrl,
      provider: repository.providerId,
      providerHost: repository.providerHost,
      providerScope: repository.providerScope,
      providerRepoId: repository.repositoryId,
      providerOwner: repository.ownerOrProject,
      providerName: repository.repositoryName,
      fullName: remoteRepositoryFullName(inspection),
    },
  ];
}

function resetDiscoveryState(resetters: FormResetters, iv?: TaskCreateDialogInitialValues) {
  const remoteRepositories = seededRemoteRepositories(iv);
  resetters.setDiscoveredRepositories([]);
  resetters.setDiscoverReposLoaded(false);
  resetters.setUseRemote(remoteRepositories.length > 0);
  resetters.setRemoteRepos(remoteRepositories);
  resetters.setGitHubUrlError(null);
  resetters.setFreshBranchEnabled(false);
  resetters.setCurrentLocalBranch("");
  // The dialog stays mounted between opens, so without this the previous
  // create's predecessor selection reappears on the next one.
  resetters.setBlockedBy([]);
}
