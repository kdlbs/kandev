import { useCallback, useEffect, useRef } from "react";
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

export function useRepositoryBranchPolicies(repositoryId: string | null, enabled = true) {
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
  const remove = useAppStore((state) => state.removeRepositoryBranchPolicy);

  const refresh = useCallback(async () => {
    if (!enabled || !repositoryId) return;
    setLoading(repositoryId, true);
    const requestRevision = revisionRef.current;
    try {
      const response = await listRepositoryBranchPolicies(repositoryId, { cache: "no-store" });
      setPolicies(repositoryId, response.repository_branch_policies, requestRevision);
    } finally {
      setLoading(repositoryId, false);
    }
  }, [enabled, repositoryId, setLoading, setPolicies]);

  useEffect(() => {
    if (!enabled || !repositoryId || isLoaded) return;
    let cancelled = false;
    setLoading(repositoryId, true);
    const requestRevision = revisionRef.current;
    listRepositoryBranchPolicies(repositoryId, { cache: "no-store" })
      .then((response) => {
        if (!cancelled) {
          setPolicies(repositoryId, response.repository_branch_policies, requestRevision);
        }
      })
      .catch(() => undefined)
      .finally(() => {
        if (!cancelled) setLoading(repositoryId, false);
      });
    return () => {
      cancelled = true;
    };
  }, [enabled, isLoaded, repositoryId, setLoading, setPolicies]);

  const create = useCallback(
    async (draft: PolicyDraft) => {
      // i18n-exempt: internal validation error
      if (!repositoryId) throw new Error("Repository is required");
      const policy = await createRepositoryBranchPolicy(repositoryId, draft);
      upsert(policy);
      return policy;
    },
    [repositoryId, upsert],
  );
  const update = useCallback(
    async (policyId: string, draft: Partial<PolicyDraft>) => {
      const policy = await updateRepositoryBranchPolicy(policyId, draft);
      upsert(policy);
      return policy;
    },
    [upsert],
  );
  const removePolicy = useCallback(
    async (policyId: string) => {
      await deleteRepositoryBranchPolicy(policyId);
      if (repositoryId) remove(repositoryId, policyId);
    },
    [remove, repositoryId],
  );
  const seedGitflow = useCallback(
    async (productionBranch: string, developmentBranch: string) => {
      // i18n-exempt: internal validation error
      if (!repositoryId) throw new Error("Repository is required");
      const response = await createGitflowRepositoryBranchPolicies(repositoryId, {
        productionBranch,
        developmentBranch,
      });
      setPolicies(repositoryId, response.repository_branch_policies);
      return response.repository_branch_policies;
    },
    [repositoryId, setPolicies],
  );

  return { policies, isLoading, refresh, create, update, remove: removePolicy, seedGitflow };
}
