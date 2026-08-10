"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useGitLabEnabled } from "@/hooks/domains/gitlab/use-gitlab-enabled";

/** Enable/disable slider for the GitLab integration, wired to `useGitLabEnabled`. */
export function GitLabEnabledControl() {
  const { enabled, setEnabled } = useGitLabEnabled();
  return (
    <DraftedIntegrationEnabledControl
      id="gitlab"
      name="GitLab"
      enabled={enabled}
      persist={setEnabled}
    />
  );
}
