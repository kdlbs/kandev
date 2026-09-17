"use client";

import { useMemo } from "react";
import type { ExecutorProfile } from "@/lib/types/http";
import type {
  DialogComputedArgs,
  DialogComputedValues,
  DialogFormState,
  AgentCompatState,
} from "@/components/task-create-dialog-types";
import {
  useRepositoryOptions,
  useBranchOptions,
  useAgentProfileOptions,
  useExecutorHint,
  useExecutorProfileOptions,
  useIsLocalExecutor,
} from "@/components/task-create-dialog-options";
import { computePassthroughProfile } from "@/components/task-create-dialog-helpers";
import {
  computeDialogDefaultStepId,
  resolveEffectiveTaskCreateWorkflowId,
} from "@/components/task-create-dialog-defaults";
import { useRemoteAuthSpecs } from "@/hooks/domains/settings/use-remote-auth-specs";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { isAgentConfiguredOnExecutor } from "@/lib/agent-executor-compat";
import type { RemoteAuthSpec } from "@/lib/api/domains/settings-api";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { isSelectableAgentProfile } from "@/lib/state/slices/settings/types";
import { getMultiRepoExecutorDisabledReason } from "@/components/task-create-dialog-multi-repo-guard";
import { resolveRepositorySelections } from "@/components/task-create-dialog-repositories-state";
import { t } from "@/lib/i18n";
import {
  deriveExecutorSourcePolicy,
  executorSourceIncompatibilityReasonKey,
  executorSourcePolicyReasonKey,
  type ExecutorSourcePolicy,
} from "@/components/task-create-dialog-executor-source-policy";
import {
  useSelectedRemoteOriginInspection,
  remoteOriginSelectionIsCompatible,
} from "@/components/task-create-dialog-remote-origin-inspection";
import type {
  RepositoryCloneSourceCandidate,
  RepositoryCloneSourceState,
} from "@/hooks/domains/repositories/use-repository-clone-source";

/**
 * Worktree executor needs a repository to create the worktree from. Disable
 * it when the task is in no-repository mode so the picker doesn't offer an
 * unworkable choice (the backend would silently fall back to local).
 */
function worktreeDisabledReason(profile: ExecutorProfile): string | null {
  if ((profile.executor_type ?? "") !== "worktree") return null;
  return t("task:worktreeExecutorRequiresRepository");
}

/**
 * Combines the two executor-disable rules into a single resolver:
 *   - no-repository mode → disable worktree (it needs a repo)
 *   - multi-repo selection → disable runtimes without sibling-repository launch support
 *   - otherwise → no disabling
 * The two never co-occur (no-repository implies zero repos, so multi-repo
 * cannot be true at the same time), so a simple priority order is enough.
 */
function pickExecutorDisabledReason(
  noRepository: boolean,
  isMultiRepoSelection: boolean,
): ((profile: ExecutorProfile) => string | null) | undefined {
  if (noRepository) return worktreeDisabledReason;
  if (isMultiRepoSelection) {
    return (profile) => getMultiRepoExecutorDisabledReason(profile.executor_type);
  }
  return undefined;
}

function resolveDialogWorkflowSelection({
  workspaceId,
  workflowId,
  selectedWorkflowId,
  lockedWorkflow,
  lastUsedWorkflowIdsByWorkspace,
  userSettingsLoaded,
  workflows,
}: Pick<
  DialogComputedArgs,
  | "workspaceId"
  | "workflowId"
  | "lockedWorkflow"
  | "lastUsedWorkflowIdsByWorkspace"
  | "userSettingsLoaded"
  | "workflows"
> & { selectedWorkflowId: string | null }) {
  const effectiveWorkflowId = resolveEffectiveTaskCreateWorkflowId({
    workspaceId,
    lockedWorkflowId: lockedWorkflow ? workflowId : null,
    manualWorkflowId: selectedWorkflowId,
    lastUsedWorkflowId:
      userSettingsLoaded === false
        ? null
        : (lastUsedWorkflowIdsByWorkspace[workspaceId ?? ""] ?? null),
    contextWorkflowId: lockedWorkflow ? null : workflowId,
    workflows,
  });
  const workflowAgentProfileId = effectiveWorkflowId
    ? (workflows.find((workflow) => workflow.id === effectiveWorkflowId)?.agent_profile_id ?? "")
    : "";
  return { effectiveWorkflowId, workflowAgentProfileId };
}

/**
 * The form has a repo selection when:
 *   - noRepository is on (intentionally repo-less), OR
 *   - useRemote is on and at least one remote-URL row has a non-empty URL, OR
 *   - useRemote is off and any workspace/local row has a repo set.
 *
 * The mode (useRemote) gates which list is consulted — rows from the
 * inactive mode are hidden but not cleared (toggle-back is non-destructive),
 * and they must not influence the submit gate.
 *
 * Exported for unit-testing the repo-selection gate independently of the
 * full `useDialogComputed` React hook.
 */
export function computeHasRepositorySelection(fs: DialogFormState): boolean {
  if (fs.noRepository) return true;
  return resolveRepositorySelections(fs).some(hasSelectedSourceValue);
}

/**
 * Number of repositories the task will operate on. Mode-aware: when Remote
 * mode is on we count non-empty URL rows, otherwise we count workspace/local
 * rows with a repo set. Rows from the inactive mode are hidden in the UI and
 * must not influence the multi-repo executor gate.
 *
 * Exported for unit-testing the executor gate independently of the React hook.
 */
export function computeSelectedRepoCount(fs: DialogFormState): number {
  if (fs.noRepository) return 0;
  return resolveRepositorySelections(fs).filter(isSelectedRepository).length;
}

function hasSelectedSourceValue(selection: ReturnType<typeof resolveRepositorySelections>[number]) {
  if (selection.kind === "remote") return selection.url.trim() !== "";
  if (selection.kind === "folder") return Boolean(selection.localPath.trim());
  return Boolean(selection.repositoryId || selection.localPath);
}

function isSelectedRepository(selection: ReturnType<typeof resolveRepositorySelections>[number]) {
  return selection.kind !== "folder" && hasSelectedSourceValue(selection);
}

/** Filter raw store profiles before executor compatibility or autopick runs. */
export function filterCompatibleAgentProfiles(
  agentProfiles: AgentProfileOption[],
  selectedExecutorProfile: ExecutorProfile | null,
  authLoaded: boolean,
  authSpecs: RemoteAuthSpec[],
  dynamicRoutingEnabled = true,
): AgentProfileOption[] {
  const selectable = agentProfiles.filter((profile) =>
    isSelectableAgentProfile(profile, dynamicRoutingEnabled),
  );
  if (!selectedExecutorProfile || !authLoaded) return selectable;
  return selectable.filter((profile) =>
    isAgentConfiguredOnExecutor(profile, selectedExecutorProfile, authSpecs),
  );
}

export type AgentCompatInput = {
  selectedExecutorProfile: ExecutorProfile | null;
  compatibleAgentProfiles: AgentProfileOption[];
  /** Effective selection: the user's pick or the workflow override. */
  selectedAgentProfileId: string;
  /** The selected profile's store row, or null when the id is unknown. */
  selectedAgentProfile: AgentProfileOption | null;
  workflowAgentLocked: boolean;
  dynamicRoutingEnabled: boolean;
};

/**
 * Compatibility of the effective agent selection with the selected
 * executor profile. Pure so the dialog's empty state, note, and footer reason
 * can be tested without rendering.
 *
 * A workflow-locked profile that fails only the credential check keeps the
 * `selected-incompatible` state even when nothing else is compatible: the
 * user cannot change the agent, so the note has to name the workflow and the
 * agent rather than the generic empty state. A locked profile that is not
 * selectable at all (disabled) is not a credential problem and stays on the
 * empty state when locked. An unlocked unavailable selection is distinct while
 * an enabled alternative exists, so the dialog does not claim that no profile
 * is compatible during automatic replacement.
 */
export function computeAgentCompatState(input: AgentCompatInput): AgentCompatState {
  const { selectedExecutorProfile, compatibleAgentProfiles, selectedAgentProfileId } = input;
  if (!selectedExecutorProfile) return "compatible";
  if (!selectedAgentProfileId) {
    return compatibleAgentProfiles.length === 0 ? "none-compatible" : "compatible";
  }
  if (compatibleAgentProfiles.some((ap) => ap.id === selectedAgentProfileId)) return "compatible";
  const selectable = input.selectedAgentProfile
    ? isSelectableAgentProfile(input.selectedAgentProfile, input.dynamicRoutingEnabled)
    : true;
  if (!selectable) {
    if (input.workflowAgentLocked || compatibleAgentProfiles.length === 0) {
      return "none-compatible";
    }
    return "selected-unavailable";
  }
  if (input.workflowAgentLocked) return "selected-incompatible";
  return compatibleAgentProfiles.length === 0 ? "none-compatible" : "selected-incompatible";
}

type AgentSelectionInput = {
  /** Effective selection: the user's pick or the workflow override. */
  agentProfileId: string;
  agentProfiles: DialogComputedArgs["agentProfiles"];
  workflowAgentLocked: boolean;
};

function useExecutorProfileCompat(
  allExecutorProfiles: ExecutorProfile[],
  selectedProfileId: string,
  selection: AgentSelectionInput,
  disabledReasonFor?: (profile: ExecutorProfile) => string | null,
) {
  const { agentProfileId: selectedAgentProfileId, agentProfiles, workflowAgentLocked } = selection;
  const executorProfileOptions = useExecutorProfileOptions(allExecutorProfiles, {
    disabledReasonFor,
  });
  const selectedExecutorProfile = useMemo(
    () => allExecutorProfiles.find((p) => p.id === selectedProfileId) ?? null,
    [allExecutorProfiles, selectedProfileId],
  );
  const { specs: authSpecs, loaded: authLoaded } = useRemoteAuthSpecs();
  const dynamicRoutingEnabled = useFeature("dynamicAgentRouting");
  const compatibleAgentProfiles = useMemo(() => {
    return filterCompatibleAgentProfiles(
      agentProfiles,
      selectedExecutorProfile,
      authLoaded,
      authSpecs,
      dynamicRoutingEnabled,
    );
  }, [agentProfiles, selectedExecutorProfile, authSpecs, authLoaded, dynamicRoutingEnabled]);
  // The agent column renders per state and the submit gate reads the derived
  // boolean, so both "no compatible agent at all" and "selected agent is not
  // compatible" block submission while only the former hides the selector.
  const selectedAgentProfile = useMemo(
    () => agentProfiles.find((ap) => ap.id === selectedAgentProfileId) ?? null,
    [agentProfiles, selectedAgentProfileId],
  );
  const agentCompatState = useMemo(
    () =>
      computeAgentCompatState({
        selectedExecutorProfile,
        compatibleAgentProfiles,
        selectedAgentProfileId,
        selectedAgentProfile,
        workflowAgentLocked,
        dynamicRoutingEnabled,
      }),
    [
      selectedExecutorProfile,
      compatibleAgentProfiles,
      selectedAgentProfileId,
      selectedAgentProfile,
      workflowAgentLocked,
      dynamicRoutingEnabled,
    ],
  );
  const noCompatibleAgent = agentCompatState !== "compatible";
  const selectedAgentProfileName = selectedAgentProfile?.label ?? null;
  return {
    selectedExecutorProfile,
    compatibleAgentProfiles,
    authLoaded,
    executorProfileOptions,
    noCompatibleAgent,
    agentCompatState,
    selectedAgentProfileName,
  };
}

function resolveExecutorSourcePolicyReason(
  inspectionPending: boolean,
  reasonKey: ReturnType<typeof executorSourceIncompatibilityReasonKey>,
): string | null {
  if (inspectionPending) return t("task:checkingRepositoryOrigin");
  if (reasonKey) return t(reasonKey);
  return null;
}

function remoteOriginInspectionPending(
  enabled: boolean,
  candidates: RepositoryCloneSourceCandidate[],
  states: Record<string, RepositoryCloneSourceState>,
): boolean {
  return (
    enabled &&
    candidates.some((candidate) => {
      const state = states[candidate.key];
      return !state || state.status === "checking";
    })
  );
}

function remoteOriginInspectionInvalid(
  fs: DialogFormState,
  enabled: boolean,
  candidates: RepositoryCloneSourceCandidate[],
  states: Record<string, RepositoryCloneSourceState>,
): boolean {
  if (!enabled) return false;
  const selections = resolveRepositorySelections(fs);
  return candidates.some((candidate) => {
    const selection = selections.find(
      (item): item is Extract<typeof item, { kind: "local" }> =>
        item.kind === "local" && item.key === candidate.key,
    );
    return !selection || !remoteOriginSelectionIsCompatible(selection, states[candidate.key]);
  });
}

function useDialogExecutorState({
  fs,
  workspaceId,
  agentProfiles,
  executors,
  repositories,
  effectiveAgentProfileId,
  workflowAgentLocked,
  agentProfileRecentUseContext,
}: {
  fs: DialogFormState;
  workspaceId: string | null;
  agentProfiles: DialogComputedArgs["agentProfiles"];
  executors: DialogComputedArgs["executors"];
  repositories: DialogComputedArgs["repositories"];
  effectiveAgentProfileId: string;
  workflowAgentLocked: boolean;
  agentProfileRecentUseContext: DialogComputedArgs["agentProfileRecentUseContext"];
}) {
  const allExecutorProfiles = useMemo<ExecutorProfile[]>(() => {
    return executors.flatMap((executor) =>
      (executor.profiles ?? []).map((profile) => ({
        ...profile,
        executor_type: profile.executor_type ?? executor.type,
        executor_name: profile.executor_name ?? executor.name,
      })),
    );
  }, [executors]);
  const selectedRepoCount = computeSelectedRepoCount(fs);
  const selectedSources = resolveRepositorySelections(fs).filter(hasSelectedSourceValue);
  const selectedSourceCount = selectedSources.length;
  const selectedFolderCount = selectedSources.filter(
    (selection) => selection.kind === "folder",
  ).length;
  const selectedLocalRepositoryCount = selectedSources.filter(
    (selection) => selection.kind === "local",
  ).length;
  const selectedRemoteOriginRepositoryCount = selectedSources.filter(
    (selection) => selection.kind === "local" && selection.checkoutSource === "remote_origin",
  ).length;
  const isMultiRepoSelection = selectedRepoCount > 1;
  const exec = useExecutorProfileCompat(
    allExecutorProfiles,
    fs.executorProfileId,
    { agentProfileId: effectiveAgentProfileId, agentProfiles, workflowAgentLocked },
    pickExecutorDisabledReason(fs.noRepository, isMultiRepoSelection),
  );
  const remoteOriginInspection = useSelectedRemoteOriginInspection(
    fs,
    workspaceId,
    exec.selectedExecutorProfile?.executor_type,
  );
  const agentProfileOptions = useAgentProfileOptions(
    exec.compatibleAgentProfiles,
    agentProfileRecentUseContext,
  );
  const defaultExecutorHint = useExecutorHint(
    executors,
    fs.executorId,
    selectedRepoCount,
    selectedSourceCount,
    selectedFolderCount,
  );
  const sourceState = useExecutorSourceState({
    fs,
    executors,
    executorId: fs.executorId,
    selectedExecutorProfile: exec.selectedExecutorProfile,
    selectedRepoCount,
    selectedSourceCount,
    selectedFolderCount,
    selectedLocalRepositoryCount,
    selectedRemoteOriginRepositoryCount,
    defaultExecutorHint,
    folderOnlyExecutorNotice: Boolean(fs.folderOnlyExecutorNotice),
    remoteOriginCandidates: remoteOriginInspection.candidates,
    remoteOriginStates: remoteOriginInspection.states,
    remoteOriginInspectionEnabled: remoteOriginInspection.enabled,
  });
  const { headerRepositoryOptions } = useRepositoryOptions(repositories, fs.discoveredRepositories);
  return {
    exec,
    agentProfileOptions,
    remoteOriginStates: remoteOriginInspection.states,
    refreshRemoteOrigins: remoteOriginInspection.refresh,
    ...sourceState,
    headerRepositoryOptions,
  };
}

function useExecutorSourceState({
  fs,
  executors,
  executorId,
  selectedExecutorProfile,
  selectedRepoCount,
  selectedSourceCount,
  selectedFolderCount,
  selectedLocalRepositoryCount,
  selectedRemoteOriginRepositoryCount,
  defaultExecutorHint,
  folderOnlyExecutorNotice,
  remoteOriginCandidates,
  remoteOriginStates,
  remoteOriginInspectionEnabled,
}: {
  fs: DialogFormState;
  executors: DialogComputedArgs["executors"];
  executorId: string;
  selectedExecutorProfile: ExecutorProfile | null;
  selectedRepoCount: number;
  selectedSourceCount: number;
  selectedFolderCount: number;
  selectedLocalRepositoryCount: number;
  selectedRemoteOriginRepositoryCount: number;
  defaultExecutorHint: string | null;
  folderOnlyExecutorNotice: boolean;
  remoteOriginCandidates: RepositoryCloneSourceCandidate[];
  remoteOriginStates: Record<string, RepositoryCloneSourceState>;
  remoteOriginInspectionEnabled: boolean;
}) {
  const isLocalExecutor = useIsLocalExecutor(executors, executorId);
  const baseExecutorSourcePolicy = useMemo<ExecutorSourcePolicy>(
    () =>
      deriveExecutorSourcePolicy({
        executorType: selectedExecutorProfile?.executor_type,
        counts: {
          sourceCount: selectedSourceCount,
          repositoryCount: selectedRepoCount,
          folderCount: selectedFolderCount,
          localRepositoryCount: selectedLocalRepositoryCount,
          remoteOriginRepositoryCount: selectedRemoteOriginRepositoryCount,
        },
      }),
    [
      selectedExecutorProfile?.executor_type,
      selectedSourceCount,
      selectedRepoCount,
      selectedFolderCount,
      selectedLocalRepositoryCount,
      selectedRemoteOriginRepositoryCount,
    ],
  );
  const remoteOriginInspectionPendingState = remoteOriginInspectionPending(
    remoteOriginInspectionEnabled,
    remoteOriginCandidates,
    remoteOriginStates,
  );
  const remoteOriginInspectionInvalidState = remoteOriginInspectionInvalid(
    fs,
    remoteOriginInspectionEnabled,
    remoteOriginCandidates,
    remoteOriginStates,
  );
  const executorSourcePolicy = useMemo<ExecutorSourcePolicy>(() => {
    if (!remoteOriginInspectionInvalidState) return baseExecutorSourcePolicy;
    return {
      ...baseExecutorSourcePolicy,
      incompatible: true,
      incompatibleReason: "repository_origin_unavailable",
    };
  }, [baseExecutorSourcePolicy, remoteOriginInspectionInvalidState]);
  const executorSourceNotice = folderOnlyExecutorNotice
    ? t("task:folderOnlyExecutorSwitched")
    : null;
  const folderPolicyReasonKey = executorSourcePolicyReasonKey(
    executorSourcePolicy.folderDisabledReason,
  );
  const folderDisabledReason = folderPolicyReasonKey ? t(folderPolicyReasonKey) : undefined;
  const sourcePolicyReasonKey = executorSourceIncompatibilityReasonKey(
    executorSourcePolicy.incompatibleReason,
  );
  const sourcePolicyReason = resolveExecutorSourcePolicyReason(
    remoteOriginInspectionPendingState,
    sourcePolicyReasonKey,
  );
  const executorHint =
    executorSourcePolicy.capabilities.requiresCloneableLocalRepository &&
    selectedRemoteOriginRepositoryCount > 0
      ? t("task:executorHintRemoteOrigin")
      : defaultExecutorHint;
  return {
    executorHint,
    executorSourcePolicy,
    executorSourceNotice,
    folderDisabledReason,
    sourcePolicyReason,
    isLocalExecutor,
  };
}

export function useDialogComputed({
  fs,
  open,
  workspaceId,
  workflowId,
  defaultStepId,
  settingsData,
  agentProfiles,
  workspaces,
  executors,
  repositories,
  workflows,
  snapshots,
  lockedWorkflow,
  lastUsedWorkflowIdsByWorkspace,
  userSettingsLoaded,
  agentProfileRecentUseContext,
}: DialogComputedArgs): DialogComputedValues {
  const { effectiveWorkflowId, workflowAgentProfileId } = resolveDialogWorkflowSelection({
    selectedWorkflowId: fs.selectedWorkflowId,
    workspaceId,
    workflowId,
    lockedWorkflow,
    lastUsedWorkflowIdsByWorkspace,
    userSettingsLoaded,
    workflows,
  });
  const workflowAgentLocked = Boolean(workflowAgentProfileId);
  // fs.agentProfileId lags behind the workflow override on dialog re-open
  // (effect deps don't change), so fall back to the synchronous value.
  const effectiveAgentProfileId = fs.agentProfileId || workflowAgentProfileId;
  const isPassthroughProfile = useMemo(
    () => computePassthroughProfile(effectiveAgentProfileId, agentProfiles),
    [effectiveAgentProfileId, agentProfiles],
  );
  const effectiveDefaultStepId = computeDialogDefaultStepId({
    selectedWorkflowId: fs.selectedWorkflowId,
    workflowId,
    fetchedSteps: fs.fetchedSteps,
    defaultStepId,
    effectiveWorkflowId,
    snapshots,
  });
  const workspaceDefaults = workspaceId ? workspaces.find((ws) => ws.id === workspaceId) : null;
  const firstRemoteUrl =
    resolveRepositorySelections(fs)
      .find((selection) => selection.kind === "remote")
      ?.url.trim() ?? "";
  const hasRepositorySelection = computeHasRepositorySelection(fs);
  // Branch options are only used by the URL-mode flow now (the chip's branch
  // pill loads branches per-repo). Keep the computed value but always feed it
  // the URL branches when in URL mode — sourced from the per-URL hook cache.
  const branchOptions = useBranchOptions(fs.branchesByUrl.branches(firstRemoteUrl));
  const executorState = useDialogExecutorState({
    fs,
    workspaceId,
    agentProfiles,
    executors,
    repositories,
    effectiveAgentProfileId,
    workflowAgentLocked,
    agentProfileRecentUseContext,
  });
  // Treat the dialog as still loading agents until BOTH the agent profiles
  // (DB rows) AND the host-utility capability probe have resolved. The
  // backend reconciler renames profiles ("Claude" → "Claude Sonnet 4.6") only
  // after the probe lands, so showing the selector before then surfaces stale
  // labels missing the model badge.
  const agentProfilesLoading =
    open && (!settingsData.agentsLoaded || !settingsData.capabilitiesLoaded);
  const executorsLoading = open && !settingsData.executorsLoaded;
  return {
    isPassthroughProfile,
    effectiveWorkflowId,
    effectiveDefaultStepId,
    workspaceDefaults,
    hasRepositorySelection,
    branchOptions,
    agentProfileOptions: executorState.agentProfileOptions,
    executorProfileOptions: executorState.exec.executorProfileOptions,
    executorHint: executorState.executorHint,
    executorSourcePolicy: executorState.executorSourcePolicy,
    executorSourceNotice: executorState.executorSourceNotice,
    sourcePolicyReason: executorState.sourcePolicyReason,
    folderDisabledReason: executorState.folderDisabledReason,
    isLocalExecutor: executorState.isLocalExecutor,
    headerRepositoryOptions: executorState.headerRepositoryOptions,
    agentProfilesLoading,
    executorsLoading,
    workflowAgentLocked,
    workflowAgentProfileId,
    effectiveAgentProfileId,
    selectedExecutorProfileName: executorState.exec.selectedExecutorProfile?.name ?? null,
    compatibleAgentProfiles: executorState.exec.compatibleAgentProfiles,
    authLoaded: executorState.exec.authLoaded,
    noCompatibleAgent: executorState.exec.noCompatibleAgent,
    agentCompatState: executorState.exec.agentCompatState,
    selectedAgentProfileName: executorState.exec.selectedAgentProfileName,
    remoteOriginStates: executorState.remoteOriginStates,
    refreshRemoteOrigins: executorState.refreshRemoteOrigins,
  };
}
