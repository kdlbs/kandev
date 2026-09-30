"use client";

import { useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import type { Message, TaskSession } from "@/lib/types/http";
import type { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import { getSessionRecoveryRetry, SessionRecoveryFeedback } from "./ensure-session-error";
import { NewSessionDialog } from "./new-session-dialog";
import { SessionRecoveryCard } from "./chat/session-recovery-card";
import {
  SessionRecoveryProvider,
  useSessionComposerRecovery,
} from "./chat/session-recovery-context";

const EMPTY_MESSAGES: Message[] = [];

export function PreviewRecoveryRegion({
  viewMode,
  session,
  archived,
  ownedByChat,
  taskId,
  workspaceId,
  resumption,
}: {
  viewMode: "session" | "plan";
  session: TaskSession | null;
  archived?: boolean;
  ownedByChat: boolean;
  taskId: string;
  workspaceId?: string | null;
  resumption: ReturnType<typeof useSessionResumption>;
}) {
  if (viewMode === "plan" && session && !archived && !session.is_passthrough)
    return (
      <PreviewPlanRecovery
        session={session}
        taskId={taskId}
        workspaceId={workspaceId}
        resumption={resumption}
      />
    );
  return (
    <PreviewRecoveryFeedback
      resumption={resumption}
      ownedByChat={ownedByChat}
      workspaceId={workspaceId}
    />
  );
}

function PreviewRecoveryFeedback({
  resumption,
  ownedByChat,
  workspaceId,
}: {
  resumption: ReturnType<typeof useSessionResumption>;
  ownedByChat: boolean;
  workspaceId?: string | null;
}) {
  return (
    <SessionRecoveryFeedback
      {...resumption}
      ownedByChat={ownedByChat}
      workspaceId={workspaceId ?? null}
      onRetry={getSessionRecoveryRetry(resumption)}
      retryDisabled={
        resumption.resumptionState === "checking" || resumption.resumptionState === "resuming"
      }
    />
  );
}

/** Plan replaces the transcript, but retains the session's lower recovery region. */
export function PreviewPlanRecovery({
  session,
  taskId,
  workspaceId,
  resumption,
}: {
  session: TaskSession;
  taskId: string;
  workspaceId?: string | null;
  resumption: ReturnType<typeof useSessionResumption>;
}) {
  const messages = useAppStore((state) => state.messages.bySession[session.id] ?? EMPTY_MESSAGES);
  return (
    <SessionRecoveryProvider session={session} messages={messages} taskId={taskId} enabled>
      <PlanRecoveryCard
        sessionId={session.id}
        taskId={taskId}
        workspaceId={workspaceId}
        resumption={resumption}
      />
    </SessionRecoveryProvider>
  );
}

function PlanRecoveryCard({
  sessionId,
  taskId,
  workspaceId,
  resumption,
}: {
  sessionId: string;
  taskId: string;
  workspaceId?: string | null;
  resumption: ReturnType<typeof useSessionResumption>;
}) {
  const recovery = useSessionComposerRecovery(sessionId);
  const actions = useSessionRecoveryActions({
    taskId,
    sessionId,
    errorStamp: recovery?.model?.stamp,
  });
  const [newSessionOpen, setNewSessionOpen] = useState(false);
  if (!recovery?.model && !resumption.error && !resumption.notice && !resumption.recoveryFailure)
    return null;
  return (
    <div className="shrink-0 min-h-0 min-w-0 p-2" data-testid="preview-plan-recovery">
      {recovery?.model && (
        <SessionRecoveryCard
          model={recovery.model}
          actions={{ ...actions, busyAction: actions.busyAction ?? recovery.pending }}
          onNewSession={() => setNewSessionOpen(true)}
        />
      )}
      <PreviewRecoveryFeedback
        resumption={resumption}
        ownedByChat={Boolean(recovery?.model)}
        workspaceId={workspaceId}
      />
      <NewSessionDialog
        open={newSessionOpen}
        onOpenChange={setNewSessionOpen}
        taskId={taskId}
        workspaceId={workspaceId}
      />
    </div>
  );
}
