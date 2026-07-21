export type ReviewProvider = "github" | "gitlab";

export function resolveReviewPanelProvider(
  params: { provider?: unknown; prKey?: unknown; mrKey?: unknown },
  hasGitHubPR: boolean,
  hasGitLabMR: boolean,
): ReviewProvider | null {
  if (params.provider === "gitlab" || typeof params.mrKey === "string") return "gitlab";
  if (params.provider === "github" || typeof params.prKey === "string") return "github";
  if (hasGitHubPR) return "github";
  if (hasGitLabMR) return "gitlab";
  return null;
}
