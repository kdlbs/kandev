"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useGitLabEnabled } from "@/hooks/domains/gitlab/use-gitlab-enabled";

export function GitLabEnabledControl() {
  const { enabled, setEnabled } = useGitLabEnabled();
  return <DraftedIntegrationEnabledControl id="gitlab" enabled={enabled} persist={setEnabled} />;
}
