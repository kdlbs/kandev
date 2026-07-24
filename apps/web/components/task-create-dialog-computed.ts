"use client";

import { useMemo } from "react";
import type { ExecutorProfile } from "@/lib/types/http";
import type {
  DialogComputedArgs,
  DialogComputedValues,
  DialogFormState,
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
  computeSingleWorkflowFallbackId,
} from "@/components/task-create-dialog-defaults";
import { useRemoteAuthSpecs } from "@/hooks/domains/settings/use-remote-auth-specs";
import { isAgentConfiguredOnExecutor } from "@/lib/agent-executor-compat";
import { getMultiRepoExecutorDisabledReason } from "@/components/task-create-dialog-multi-repo-guard";

/**
 * Worktree executor needs a repository to create the worktree from. Disable
 * it when the task is in no-repository mode so the picker doesn't offer an
 * unworkable choice (the backend would silently fall back to local).
 */
function worktreeDisabledReason(profile: ExecutorProfile): string | null {
  if ((profile.executor_type ?? "") !== "worktree") return null;
  return "Worktree executor requires a repository.";
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
  if (fs.useRemote) return fs.remoteRepos.some((r) => r.url.trim() !== "");
  return fs.repositories.some((r) => r.repositoryId || r.localPath);
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
  if (fs.useRemote) return fs.remoteRepos.filter((r) => r.url.trim() !== "").length;
  return fs.repositories.filter((r) => r.repositoryId || r.localPath).length;
}

function useExecutorProfileCompat(
  allExecutorProfiles: ExecutorProfile[],
  selectedProfileId: string,
  selectedAgentProfileId: string,
  agentProfiles: DialogComputedArgs["agentProfiles"],
  disabledReasonFor?: (profile: ExecutorProfile) => string | null,
) {
  const executorProfileOptions = useExecutorProfileOptions(allExecutorProfiles, {
    disabledReasonFor,
  });
  const selectedExecutorProfile = useMemo(
    () => allExecutorProfiles.find((p) => p.id === selectedProfileId) ?? null,
    [allExecutorProfiles, selectedProfileId],
  );
  const { specs: authSpecs, loaded: authLoaded } = useRemoteAuthSpecs();
  const compatibleAgentProfiles = useMemo(() => {
    if (!selectedExecutorProfile || !authLoaded) return agentProfiles;
    return agentProfiles.filter((ap) =>
      isAgentConfiguredOnExecutor(ap, selectedExecutorProfile, authSpecs),
    );
  }, [agentProfiles, selectedExecutorProfile, authSpecs, authLoaded]);
  // `noCompatibleAgent` gates the submit button. It must catch BOTH cases:
  //   1. The selected executor has no compatible agents at all.
  //   2. The user picked an agent that isn't compatible with the executor
  //      (e.g. switched executor after the agent was chosen).
  // Previously this only checked case 1, so case 2 silently let the user
  // submit with a known-incompatible combination.
  const noCompatibleAgent = useMemo(() => {
    if (!selectedExecutorProfile) return false;
    if (compatibleAgentProfiles.length === 0) return true;
    if (!selectedAgentProfileId) return false;
    return !compatibleAgentProfiles.some((ap) => ap.id === selectedAgentProfileId);
  }, [selectedExecutorProfile, compatibleAgentProfiles, selectedAgentProfileId]);
  return {
    selectedExecutorProfile,
    compatibleAgentProfiles,
    authLoaded,
    executorProfileOptions,
    noCompatibleAgent,
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
}: DialogComputedArgs): DialogComputedValues {
  const singleWorkflowId = computeSingleWorkflowFallbackId(
    fs.selectedWorkflowId,
    workflowId,
    workflows,
  );
  const effectiveWorkflowId = fs.selectedWorkflowId ?? workflowId ?? singleWorkflowId;
  // Compute workflow agent lock directly from data — avoids effect timing issues.
  const workflowAgentProfileId = (() => {
    const wfId = effectiveWorkflowId;
    if (!wfId) return "";
    const wf = workflows.find((w) => w.id === wfId);
    return wf?.agent_profile_id ?? "";
  })();
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
  const firstRemoteUrl = fs.remoteRepos[0]?.url.trim() ?? "";
  const hasRepositorySelection = computeHasRepositorySelection(fs);
  // Branch options are only used by the URL-mode flow now (the chip's branch
  // pill loads branches per-repo). Keep the computed value but always feed it
  // the URL branches when in URL mode — sourced from the per-URL hook cache.
  const branchOptions = useBranchOptions(
    fs.useRemote ? fs.branchesByUrl.branches(firstRemoteUrl) : [],
  );
  const allExecutorProfiles = useMemo<ExecutorProfile[]>(() => {
    return executors.flatMap((executor) =>
      (executor.profiles ?? []).map((p) => ({
        ...p,
        executor_type: p.executor_type ?? executor.type,
        executor_name: p.executor_name ?? executor.name,
      })),
    );
  }, [executors]);
  // Gate only runtimes whose launch path cannot project sibling repositories.
  // Count BOTH workspace/local rows AND remote-URL rows (each non-empty row is
  // a distinct repo the task will operate on) so a 2-row Remote selection
  // applies the same capability check.
  const selectedRepoCount = computeSelectedRepoCount(fs);
  const isMultiRepoSelection = selectedRepoCount > 1;
  // Use the effective agent ID (form value OR the workflow-locked override)
  // so the compatibility gate catches the override case too — passing the
  // raw fs.agentProfileId would let workflow-locked sessions slip past with
  // an empty selection.
  const exec = useExecutorProfileCompat(
    allExecutorProfiles,
    fs.executorProfileId,
    effectiveAgentProfileId,
    agentProfiles,
    pickExecutorDisabledReason(fs.noRepository, isMultiRepoSelection),
  );
  const agentProfileOptions = useAgentProfileOptions(exec.compatibleAgentProfiles);
  const executorHint = useExecutorHint(executors, fs.executorId, selectedRepoCount);
  const isLocalExecutor = useIsLocalExecutor(executors, fs.executorId);
  const { headerRepositoryOptions } = useRepositoryOptions(repositories, fs.discoveredRepositories);
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
    agentProfileOptions,
    executorProfileOptions: exec.executorProfileOptions,
    executorHint,
    isLocalExecutor,
    headerRepositoryOptions,
    agentProfilesLoading,
    executorsLoading,
    workflowAgentLocked,
    workflowAgentProfileId,
    effectiveAgentProfileId,
    selectedExecutorProfileName: exec.selectedExecutorProfile?.name ?? null,
    compatibleAgentProfiles: exec.compatibleAgentProfiles,
    authLoaded: exec.authLoaded,
    noCompatibleAgent: exec.noCompatibleAgent,
  };
}
