import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  createGitflowRepositoryBranchPolicies,
  createRepositoryBranchPolicy,
  deleteRepositoryBranchPolicy,
  listRepositoryBranchPolicies,
  updateRepositoryBranchPolicy,
} from "@/lib/api";
import type { RepositoryBranchPolicy } from "@/lib/types/http";

const EMPTY_POLICIES: RepositoryBranchPolicy[] = [];
type PolicyDraft = Omit<
  RepositoryBranchPolicy,
  "id" | "repository_id" | "created_at" | "updated_at"
>;

type InitialPolicyLoadArgs = {
  enabled: boolean;
  isLoaded: boolean;
  repositoryId: string | null;
  revisionRef: { current: number };
  setLoading: (repositoryId: string, loading: boolean) => void;
  setError: (hasError: boolean) => void;
  setPolicies: (
    repositoryId: string,
    policies: RepositoryBranchPolicy[],
    expectedRevision?: number,
  ) => void;
};

function useInitialRepositoryBranchPolicies({
  enabled,
  isLoaded,
  repositoryId,
  revisionRef,
  setLoading,
  setError,
  setPolicies,
}: InitialPolicyLoadArgs) {
  useEffect(() => {
    if (!enabled || !repositoryId || isLoaded) return;
    let cancelled = false;
    setLoading(repositoryId, true);
    setError(false);
    const requestRevision = revisionRef.current;
    listRepositoryBranchPolicies(repositoryId, { cache: "no-store" })
      .then((response) => {
        if (!cancelled)
          setPolicies(repositoryId, response.repository_branch_policies, requestRevision);
      })
      .catch(() => {
        if (!cancelled) setError(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(repositoryId, false);
      });
    return () => {
      cancelled = true;
    };
  }, [enabled, isLoaded, repositoryId, revisionRef, setError, setLoading, setPolicies]);
}

type PolicyMutationArgs = {
  repositoryId: string | null;
  revisionRef: { current: number };
  refresh: () => Promise<void>;
  upsert: (policy: RepositoryBranchPolicy) => void;
  remove: (repositoryId: string, policyId: string) => void;
  setPolicies: InitialPolicyLoadArgs["setPolicies"];
};

function useRepositoryBranchPolicyMutations({
  repositoryId,
  revisionRef,
  refresh,
  upsert,
  remove,
  setPolicies,
}: PolicyMutationArgs) {
  const create = useCallback(
    async (draft: PolicyDraft) => {
      // i18n-exempt: internal validation error
      if (!repositoryId) throw new Error("Repository is required");
      const requestRevision = revisionRef.current;
      const policy = await createRepositoryBranchPolicy(repositoryId, draft);
      if (revisionRef.current !== requestRevision) {
        await refresh();
        return policy;
      }
      upsert(policy);
      return policy;
    },
    [refresh, repositoryId, revisionRef, upsert],
  );
  const update = useCallback(
    async (policyId: string, draft: Partial<PolicyDraft>) => {
      const requestRevision = revisionRef.current;
      const policy = await updateRepositoryBranchPolicy(policyId, draft);
      if (revisionRef.current !== requestRevision) {
        await refresh();
        return policy;
      }
      upsert(policy);
      return policy;
    },
    [refresh, revisionRef, upsert],
  );
  const removePolicy = useCallback(
    async (policyId: string) => {
      const requestRevision = revisionRef.current;
      await deleteRepositoryBranchPolicy(policyId);
      if (!repositoryId) return;
      if (revisionRef.current !== requestRevision) {
        await refresh();
        return;
      }
      remove(repositoryId, policyId);
    },
    [refresh, remove, repositoryId, revisionRef],
  );
  const seedGitflow = useCallback(
    async (productionBranch: string, developmentBranch: string) => {
      // i18n-exempt: internal validation error
      if (!repositoryId) throw new Error("Repository is required");
      const requestRevision = revisionRef.current;
      const response = await createGitflowRepositoryBranchPolicies(repositoryId, {
        productionBranch,
        developmentBranch,
      });
      if (revisionRef.current !== requestRevision) {
        await refresh();
        return response.repository_branch_policies;
      }
      setPolicies(repositoryId, response.repository_branch_policies, requestRevision);
      return response.repository_branch_policies;
    },
    [refresh, repositoryId, revisionRef, setPolicies],
  );

  return { create, update, remove: removePolicy, seedGitflow };
}

export function useRepositoryBranchPolicies(repositoryId: string | null, enabled = true) {
  const [hasError, setHasError] = useState(false);
  const policies = useAppStore((state) =>
    repositoryId
      ? (state.repositoryBranchPolicies.itemsByRepositoryId[repositoryId] ?? EMPTY_POLICIES)
      : EMPTY_POLICIES,
  );
  const isLoading = useAppStore((state) =>
    repositoryId
      ? (state.repositoryBranchPolicies.loadingByRepositoryId[repositoryId] ?? false)
      : false,
  );
  const isLoaded = useAppStore((state) =>
    repositoryId
      ? (state.repositoryBranchPolicies.loadedByRepositoryId[repositoryId] ?? false)
      : false,
  );
  const revision = useAppStore((state) =>
    repositoryId ? (state.repositoryBranchPolicies.revisionByRepositoryId[repositoryId] ?? 0) : 0,
  );
  const revisionRef = useRef(revision);
  revisionRef.current = revision;
  const setPolicies = useAppStore((state) => state.setRepositoryBranchPolicies);
  const setLoading = useAppStore((state) => state.setRepositoryBranchPoliciesLoading);
  const upsert = useAppStore((state) => state.upsertRepositoryBranchPolicy);
  const removePolicyFromStore = useAppStore((state) => state.removeRepositoryBranchPolicy);

  const refresh = useCallback(async () => {
    if (!enabled || !repositoryId) return;
    setLoading(repositoryId, true);
    setHasError(false);
    const requestRevision = revisionRef.current;
    try {
      const response = await listRepositoryBranchPolicies(repositoryId, { cache: "no-store" });
      setPolicies(repositoryId, response.repository_branch_policies, requestRevision);
    } catch (error) {
      setHasError(true);
      throw error;
    } finally {
      setLoading(repositoryId, false);
    }
  }, [enabled, repositoryId, setHasError, setLoading, setPolicies]);

  useInitialRepositoryBranchPolicies({
    enabled,
    isLoaded,
    repositoryId,
    revisionRef,
    setError: setHasError,
    setLoading,
    setPolicies,
  });
  const mutations = useRepositoryBranchPolicyMutations({
    repositoryId,
    revisionRef,
    refresh,
    upsert,
    remove: removePolicyFromStore,
    setPolicies,
  });

  return { policies, isLoading, hasError, refresh, ...mutations };
}
