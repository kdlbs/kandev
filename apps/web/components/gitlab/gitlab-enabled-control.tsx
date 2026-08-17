"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useGitLabEnabled } from "@/hooks/domains/gitlab/use-gitlab-enabled";
import type { IntegrationEnabledControlProps } from "@/components/integrations/integration-enabled-control-props";

/**
 * Enable/disable slider for the GitLab integration in `workspaceId`, wired to
 * `useGitLabEnabled`.
 */
export function GitLabEnabledControl({ workspaceId }: IntegrationEnabledControlProps) {
  const { enabled, setEnabled } = useGitLabEnabled(workspaceId);
  return (
    <DraftedIntegrationEnabledControl
      id="gitlab"
      name="GitLab"
      enabled={enabled}
      persist={setEnabled}
    />
  );
}
