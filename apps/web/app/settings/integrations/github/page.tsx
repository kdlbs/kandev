import { GitHubIntegrationPage } from "@/components/github/github-settings";
import { StateHydrator } from "@/components/state-hydrator";
import { normalizeGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { fetchGitHubStatus } from "@/lib/api/domains/github-api";

type IntegrationsGitHubPageProps = {
  workspaceId?: string;
};

export default async function IntegrationsGitHubPage({
  workspaceId,
}: IntegrationsGitHubPageProps = {}) {
  const status = workspaceId
    ? await fetchGitHubStatus(workspaceId, { cache: "no-store" }).catch(() => null)
    : null;
  const initialState = status
    ? {
        githubStatus: {
          workspaceId: workspaceId ?? status.workspace_id ?? null,
          status: normalizeGitHubStatus(status),
          loaded: true,
          loading: false,
        },
      }
    : {};
  return (
    <>
      <StateHydrator initialState={initialState} />
      <GitHubIntegrationPage workspaceId={workspaceId} />
    </>
  );
}
