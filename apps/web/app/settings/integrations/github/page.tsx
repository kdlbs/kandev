import { GitHubIntegrationPage } from "@/components/github/github-settings";
import { GitHubStatusSeed } from "@/components/github/github-status-seed";
import { fetchGitHubStatus } from "@/lib/api/domains/github-api";

export default async function IntegrationsGitHubPage() {
  const status = await fetchGitHubStatus({ cache: "no-store" }).catch(() => null);
  return (
    <>
      <GitHubStatusSeed status={status} />
      <GitHubIntegrationPage />
    </>
  );
}
