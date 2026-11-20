"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  getWorkspaceRouting,
  retryProvider,
  updateSeatAuthorExclusion,
  updateWorkspaceRouting,
} from "@/lib/api/domains/office-extended-api";
import type { ExecutionProfileSummary, WorkspaceRouting } from "@/lib/state/slices/office/types";
import type {
  AuthorExclusionStep,
  AuthorExclusionStepChange,
  AuthorExclusionWarning,
} from "@/lib/types/author-exclusion";
import { t } from "@/lib/i18n";

export type UseWorkspaceRoutingResult = {
  config: WorkspaceRouting | undefined;
  knownProviders: string[];
  executionProfiles: ExecutionProfileSummary[];
  authorExclusionWarnings: AuthorExclusionWarning[];
  /** Seat steps with their Review on a different provider setting. */
  authorExclusionSteps: AuthorExclusionStep[];
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  /** Saves cfg and resolves to the exclusion setup warnings computed after the save. */
  update: (cfg: WorkspaceRouting) => Promise<AuthorExclusionWarning[]>;
  /** Saves seat switch changes and resolves to the warnings computed after the save. */
  updateAuthorExclusion: (
    changes: AuthorExclusionStepChange[],
  ) => Promise<AuthorExclusionWarning[]>;
  retry: (providerId: string) => Promise<void>;
};

const EMPTY: never[] = [];

function useWorkspaceRoutingStore(workspaceName: string | null) {
  const config = useAppStore((s) =>
    workspaceName ? s.office.routing.byWorkspace[workspaceName] : undefined,
  );
  const knownProviders = useAppStore((s) => s.office.routing.knownProviders);
  const setWorkspaceRouting = useAppStore((s) => s.setWorkspaceRouting);
  const setKnownProviders = useAppStore((s) => s.setKnownProviders);
  return { config, knownProviders, setWorkspaceRouting, setKnownProviders };
}

export function useWorkspaceRouting(workspaceName: string | null): UseWorkspaceRoutingResult {
  const { config, knownProviders, setWorkspaceRouting, setKnownProviders } =
    useWorkspaceRoutingStore(workspaceName);
  const [isLoading, setIsLoading] = useState(false);
  const [executionProfiles, setExecutionProfiles] = useState<ExecutionProfileSummary[]>([]);
  const [authorExclusionWarnings, setAuthorExclusionWarnings] = useState<AuthorExclusionWarning[]>(
    [],
  );
  const [authorExclusionSteps, setAuthorExclusionSteps] = useState<AuthorExclusionStep[]>([]);
  const [profilesWorkspace, setProfilesWorkspace] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const requestVersion = useRef(0);
  const currentWorkspace = useRef(workspaceName);
  currentWorkspace.current = workspaceName;

  const refresh = useCallback(async () => {
    if (!workspaceName) return;
    const version = ++requestVersion.current;
    setIsLoading(true);
    setError(null);
    try {
      const res = await getWorkspaceRouting(workspaceName);
      if (version !== requestVersion.current) return;
      if (res.config) setWorkspaceRouting(workspaceName, res.config);
      if (Array.isArray(res.known_providers)) setKnownProviders(res.known_providers);
      setExecutionProfiles(Array.isArray(res.execution_profiles) ? res.execution_profiles : []);
      setAuthorExclusionWarnings(res.author_exclusion_warnings ?? []);
      setAuthorExclusionSteps(res.author_exclusion_steps ?? []);
      setProfilesWorkspace(workspaceName);
    } catch (e) {
      if (version !== requestVersion.current) return;
      setError(e instanceof Error ? e.message : t("office:failedToLoadRoutingConfig"));
    } finally {
      if (version === requestVersion.current) setIsLoading(false);
    }
  }, [workspaceName, setWorkspaceRouting, setKnownProviders]);

  useEffect(() => {
    if (!workspaceName) {
      requestVersion.current += 1;
      setExecutionProfiles([]);
      setAuthorExclusionWarnings([]);
      setAuthorExclusionSteps([]);
      setProfilesWorkspace(null);
      setIsLoading(false);
      return;
    }
    if (profilesWorkspace === workspaceName) return;
    void refresh();
  }, [workspaceName, profilesWorkspace, refresh]);

  const update = useCallback(
    async (cfg: WorkspaceRouting) => {
      if (!workspaceName) return [];
      const res = await updateWorkspaceRouting(workspaceName, cfg);
      setWorkspaceRouting(workspaceName, cfg);
      const warnings = res?.author_exclusion_warnings ?? [];
      if (currentWorkspace.current === workspaceName) setAuthorExclusionWarnings(warnings);
      return warnings;
    },
    [workspaceName, setWorkspaceRouting],
  );

  const updateAuthorExclusion = useCallback(
    async (changes: AuthorExclusionStepChange[]) => {
      if (!workspaceName || changes.length === 0) return [];
      const res = await updateSeatAuthorExclusion(workspaceName, changes);
      const warnings = res.author_exclusion_warnings ?? [];
      if (currentWorkspace.current !== workspaceName) return warnings;
      setAuthorExclusionWarnings(warnings);
      const steps = res.author_exclusion_steps;
      // Keep the acknowledged values even if the follow-up read fails.
      setAuthorExclusionSteps((current) => steps ?? applySavedSeatChanges(current, changes));
      if (steps) {
        // This saved list supersedes any refresh that is still in flight.
        requestVersion.current += 1;
        setIsLoading(false);
        setError(null);
      } else void refresh();
      return warnings;
    },
    [workspaceName, refresh],
  );

  const retry = useCallback(
    async (providerId: string) => {
      if (!workspaceName) return;
      await retryProvider(workspaceName, providerId);
    },
    [workspaceName],
  );

  // Per-workspace data from an earlier workspace is never shown, even while
  // the new workspace's read is in flight or after it fails.
  const loadedForWorkspace = workspaceName !== null && profilesWorkspace === workspaceName;
  return {
    config,
    knownProviders,
    executionProfiles: loadedForWorkspace ? executionProfiles : EMPTY,
    authorExclusionWarnings: loadedForWorkspace ? authorExclusionWarnings : EMPTY,
    authorExclusionSteps: loadedForWorkspace ? authorExclusionSteps : EMPTY,
    isLoading,
    error,
    refresh,
    update,
    updateAuthorExclusion,
    retry,
  };
}

function applySavedSeatChanges(steps: AuthorExclusionStep[], changes: AuthorExclusionStepChange[]) {
  return steps.map((step) => {
    const saved = changes.findLast(
      (change) => change.step_id === step.step_id && change.role === step.role,
    );
    return saved ? { ...step, enabled: saved.enabled } : step;
  });
}
