"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useGitHubEnabled } from "@/hooks/domains/github/use-github-enabled";

export function GitHubEnabledControl() {
  const { enabled, setEnabled } = useGitHubEnabled();
  return <DraftedIntegrationEnabledControl id="github" enabled={enabled} persist={setEnabled} />;
}
