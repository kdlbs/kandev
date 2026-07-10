"use client";

import { useJiraAvailable } from "@/hooks/domains/jira/use-jira-availability";
import { useLinearAvailable } from "@/hooks/domains/linear/use-linear-availability";
import { useSentryAvailable } from "@/hooks/domains/sentry/use-sentry-availability";

export type KanbanExternalLinkAvailability = {
  jira: boolean;
  linear: boolean;
  sentry: boolean;
};

export function useKanbanExternalLinkAvailability(
  workspaceId: string | null,
): KanbanExternalLinkAvailability {
  return {
    jira: useJiraAvailable(workspaceId),
    linear: useLinearAvailable(workspaceId),
    sentry: useSentryAvailable(workspaceId),
  };
}
