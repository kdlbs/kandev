"use client";

import { useJiraAvailable } from "@/hooks/domains/jira/use-jira-availability";
import { useGitLabAvailable } from "@/hooks/domains/gitlab/use-task-mr";
import { useLinearAvailable } from "@/hooks/domains/linear/use-linear-availability";
import { useSentryAvailable } from "@/hooks/domains/sentry/use-sentry-availability";

export type KanbanExternalLinkAvailability = {
  gitlab?: boolean;
  jira: boolean;
  linear: boolean;
  sentry: boolean;
};

export function useKanbanExternalLinkAvailability(
  workspaceId: string | null,
): KanbanExternalLinkAvailability {
  return {
    gitlab: useGitLabAvailable(),
    jira: useJiraAvailable(workspaceId),
    linear: useLinearAvailable(workspaceId),
    sentry: useSentryAvailable(workspaceId),
  };
}
