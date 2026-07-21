import { GitHubIntegrationPage } from "@/components/github/github-settings";
import { StateHydrator } from "@/components/state-hydrator";
import { fetchGitHubStatus } from "@/lib/api/domains/github-api";

type IntegrationsGitHubPageProps = {
  workspaceId?: string;
};

export default async function IntegrationsGitHubPage({
  workspaceId,
}: IntegrationsGitHubPageProps = {}) {
  const status = await fetchGitHubStatus({ cache: "no-store" }).catch(() => null);
  const initialState = status
    ? {
        githubStatus: {
          status,
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
