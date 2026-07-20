import { GitHubAppSettings } from "@/components/settings/system/github-app-settings";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";

export default function GitHubAppPage() {
  return (
    <SystemPageShell
      title="GitHub App"
      description="Create and monitor the deployment-owned GitHub App used for workspace automation."
    >
      <GitHubAppSettings />
    </SystemPageShell>
  );
}
