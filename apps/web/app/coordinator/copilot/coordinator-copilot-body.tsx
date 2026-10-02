import { ActivityDisplayContext } from "@/components/task/chat/messages/agent-status-mode";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { QuickChatSessionView } from "@/components/quick-chat/quick-chat-session-view";
import { SessionRecoveryFeedback } from "@/components/task/ensure-session-error";
import { MessageTaskOriginProvider } from "@/components/task/chat/messages/message-task-origin-context";
import { isTerminalSessionState } from "@/lib/ws/handlers/agent-session";
import { CoordinatorProposalProvider } from "@/components/task/chat/messages/kandev/coordinator-proposal-context";
import type { CopilotChip } from "@/hooks/domains/coordinator/copilot-store";
import type { OpenSequenceState } from "@/hooks/domains/coordinator/use-copilot-open-sequence";
import type { ConversationResponse } from "@/lib/api/domains/coordinator-api";
import type { QuickChatSession } from "@/lib/state/slices/ui/types";
import { profileStatusMessages } from "./profile-messages";
import {
  CoordinatorCopilotChipRow,
  CoordinatorCopilotEmptyIntro,
} from "./coordinator-copilot-chip";

const RETRYABLE_ERRORS = {
  "load-failed": "coordinator:copilotLoadFailedMessage",
  conflict: "coordinator:copilotConflictMessage",
  "open-failed": "coordinator:copilotOpenFailedMessage",
} as const;

function ProfileMessages({
  agentStatus,
  executorStatus,
}: {
  agentStatus: string;
  executorStatus: string;
}) {
  const { t } = useTranslation();
  const messages = profileStatusMessages(agentStatus, executorStatus, t);
  return (
    <div
      className="space-y-2 p-4 text-sm text-muted-foreground"
      data-testid="copilot-profile-messages"
    >
      {messages.agentMessage && <p>{messages.agentMessage}</p>}
      {messages.executorMessage && <p>{messages.executorMessage}</p>}
    </div>
  );
}

function RetryableError({ messageKey, onRetry }: { messageKey: string; onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2 p-4 text-sm text-muted-foreground" data-testid="copilot-open-error">
      <p>{t(messageKey)}</p>
      <Button variant="outline" size="sm" className="cursor-pointer" onClick={onRetry}>
        {t("coordinator:tryAgain")}
      </Button>
    </div>
  );
}

function GoneMessage() {
  const { t } = useTranslation();
  return (
    <div className="p-4 text-sm text-muted-foreground" data-testid="copilot-gone-message">
      {t("coordinator:copilotGoneMessage")}
    </div>
  );
}

function ReadyBody({
  routeSession,
  workspaceId,
  coordinatorId,
  chip,
  pendingDraft,
  askKey,
  onRemoveChip,
  onSuggest,
  onRetry,
  retryDisabled,
  onClosePopover,
}: {
  routeSession: ConversationResponse;
  workspaceId: string;
  coordinatorId: string;
  chip: CopilotChip | null;
  pendingDraft: string | undefined;
  askKey: number;
  onRemoveChip: () => void;
  onSuggest: (text: string) => void;
  onRetry: () => void;
  retryDisabled: boolean;
  onClosePopover: () => void;
}) {
  const { t } = useTranslation();
  const isEmpty = useAppStore(
    (state) => (state.messages.bySession[routeSession.session_id]?.length ?? 0) === 0,
  );
  const sessionState = useAppStore(
    (state) => state.taskSessions.items[routeSession.session_id]?.state,
  );
  const errorMessage = useAppStore(
    (state) => state.taskSessions.items[routeSession.session_id]?.error_message,
  );
  const ended = isTerminalSessionState(sessionState);
  const session: QuickChatSession = {
    kind: "chat",
    sessionId: routeSession.session_id,
    workspaceId,
    taskId: routeSession.task_id,
  };
  // i18n-exempt: wire prefix parsed back by parseCoordinatorAboutPrefix (user-message-body.tsx); never translated.
  const transformOutgoing = chip
    ? (message: string) => `About ${chip.id} [${chip.ref.kind}:${chip.ref.id}]: ${message}`
    : undefined;
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {chip && <CoordinatorCopilotChipRow chip={chip} onRemove={onRemoveChip} />}
      {ended && (
        <SessionRecoveryFeedback
          error={errorMessage?.trim() || t("task:backendRejectedSessionRequest")}
          notice={null}
          onRetry={onRetry}
          workspaceId={workspaceId}
          retryDisabled={retryDisabled}
          testId="session-recovery-error"
        />
      )}
      {isEmpty && !ended && <CoordinatorCopilotEmptyIntro onSuggest={onSuggest} />}
      <MessageTaskOriginProvider value="coordinator">
        <CoordinatorProposalProvider
          value={{ workspaceId, coordinatorId, closePopover: onClosePopover }}
        >
          <ActivityDisplayContext.Provider value>
            <QuickChatSessionView
              key={`${routeSession.session_id}-${askKey}`}
              session={session}
              automaticRecovery={false}
              hideSessionSelectors
              hideStartupRows
              activityDisplay
              taskArchiveState={routeSession.archive_state}
              initialDraft={pendingDraft}
              transformOutgoing={transformOutgoing}
            />
          </ActivityDisplayContext.Provider>
        </CoordinatorProposalProvider>
      </MessageTaskOriginProvider>
    </div>
  );
}

export type CoordinatorCopilotBodyProps = {
  workspaceId: string;
  coordinatorId: string;
  state: OpenSequenceState;
  routeSession: ConversationResponse | null;
  chip: CopilotChip | null;
  pendingDraft: string | undefined;
  askKey: number;
  onRetry: () => void;
  onRemoveChip: () => void;
  onSuggest: (text: string) => void;
  onClosePopover: () => void;
};

/** Switches the popover body on the open sequence's outcome
 *  (docs/specs/coordinator/system-design/copilot-popover.md#opening-the-conversation).
 *  A freshly-resolved profile-unavailable, gone, or error outcome always
 *  replaces the body. Otherwise `ReadyBody` renders whenever a route session
 *  is held, including while a same-coordinator revalidation (Ask about
 *  this, Try again) is in flight, so the transcript and composer stay
 *  mounted across it. */
export function CoordinatorCopilotBody({
  workspaceId,
  coordinatorId,
  state,
  routeSession,
  chip,
  pendingDraft,
  askKey,
  onRetry,
  onRemoveChip,
  onSuggest,
  onClosePopover,
}: CoordinatorCopilotBodyProps) {
  if (state.kind === "profile-unavailable") {
    return (
      <ProfileMessages agentStatus={state.agentStatus} executorStatus={state.executorStatus} />
    );
  }
  if (state.kind === "gone") return <GoneMessage />;
  if (state.kind === "error") {
    return <RetryableError messageKey={RETRYABLE_ERRORS[state.error]} onRetry={onRetry} />;
  }
  if (routeSession) {
    return (
      <ReadyBody
        routeSession={routeSession}
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
        chip={chip}
        pendingDraft={pendingDraft}
        askKey={askKey}
        onRemoveChip={onRemoveChip}
        onSuggest={onSuggest}
        onRetry={onRetry}
        retryDisabled={state.kind === "loading"}
        onClosePopover={onClosePopover}
      />
    );
  }
  return null;
}
