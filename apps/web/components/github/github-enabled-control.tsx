"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useGitHubEnabled } from "@/hooks/domains/github/use-github-enabled";
import type { IntegrationEnabledControlProps } from "@/components/integrations/integration-enabled-control-props";

/**
 * Enable/disable slider for the GitHub integration in `workspaceId`, wired to
 * `useGitHubEnabled`.
 */
export function GitHubEnabledControl({ workspaceId }: IntegrationEnabledControlProps) {
  const { enabled, setEnabled } = useGitHubEnabled(workspaceId);
  return (
    <DraftedIntegrationEnabledControl
      id="github"
      name="GitHub"
      enabled={enabled}
      persist={setEnabled}
    />
  );
}
