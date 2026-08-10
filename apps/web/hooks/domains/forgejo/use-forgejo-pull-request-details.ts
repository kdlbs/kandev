import { useCallback, useState } from "react";
import { createForgejoPullRequestComment, getForgejoPullRequestDetails, submitForgejoPullRequestReview } from "@/lib/api/domains/forgejo-api";
import type { ForgejoPullRequestDetails } from "@/lib/types/forgejo";

export function useForgejoPullRequestDetails(workspaceId: string) {
  const [details, setDetails] = useState<ForgejoPullRequestDetails | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const load = useCallback(async (owner: string, repo: string, number: number) => {
    setLoading(true); setError(null);
    try { setDetails(await getForgejoPullRequestDetails(owner, repo, number, { workspaceId })); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Could not load pull request details"); }
    finally { setLoading(false); }
  }, [workspaceId]);
  const comment = useCallback(async (owner: string, repo: string, number: number, body: string) => {
    await createForgejoPullRequestComment({ owner, repo, number, body }, { workspaceId });
    await load(owner, repo, number);
  }, [load, workspaceId]);
  const review = useCallback(async (owner: string, repo: string, number: number, event: "APPROVE" | "REQUEST_CHANGES" | "COMMENT", body?: string) => {
    await submitForgejoPullRequestReview({ owner, repo, number, event, body }, { workspaceId });
    await load(owner, repo, number);
  }, [load, workspaceId]);
  return { details, loading, error, load, comment, review };
}
