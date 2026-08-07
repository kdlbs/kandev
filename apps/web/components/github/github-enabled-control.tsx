"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useGitHubEnabled } from "@/hooks/domains/github/use-github-enabled";

/** Enable/disable slider for the GitHub integration, wired to `useGitHubEnabled`. */
export function GitHubEnabledControl() {
  const { enabled, setEnabled } = useGitHubEnabled();
  return (
    <DraftedIntegrationEnabledControl
      id="github"
      name="GitHub"
      enabled={enabled}
      persist={setEnabled}
    />
  );
}
