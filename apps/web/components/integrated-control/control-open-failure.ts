import { profileStatusMessages } from "@/app/coordinator/copilot/profile-messages";
import type { OpenSequenceState } from "@/hooks/domains/coordinator/use-copilot-open-sequence";

type Translate = (key: string) => string;

const ERROR_KEYS = {
  "load-failed": "coordinator:copilotLoadFailedMessage",
  conflict: "coordinator:copilotConflictMessage",
  "open-failed": "coordinator:copilotOpenFailedMessage",
} as const;

/**
 * Why opening a coordinator conversation failed, in the coordinator popover's
 * own words: one line per unusable profile, the removed-coordinator message,
 * or the matching retryable error.
 */
export function openFailureMessages(state: OpenSequenceState | null, t: Translate): string[] {
  if (!state) return [];
  if (state.kind === "profile-unavailable") {
    const { agentMessage, executorMessage } = profileStatusMessages(
      state.agentStatus,
      state.executorStatus,
      t,
    );
    return [agentMessage, executorMessage].filter((text): text is string => Boolean(text));
  }
  if (state.kind === "gone") return [t("coordinator:copilotGoneMessage")];
  if (state.kind === "error") return [t(ERROR_KEYS[state.error])];
  return [];
}
