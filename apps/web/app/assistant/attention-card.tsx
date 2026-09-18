import { useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { Button } from "@kandev/ui/button";
import { ClarificationInputOverlay } from "@/components/task/chat/clarification-input-overlay";
import { ClarificationTransportContext } from "@/components/task/chat/clarification-transport";
import { NativePermissionOptions } from "@/components/task/chat/messages/permission-action-row";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/ids";
import type { Message } from "@/lib/types/http";
import type {
  AssistantAttention,
  AssistantBinding,
  AssistantInputSnapshot,
} from "@/lib/api/domains/assistant-api";
import { useAssistantInput } from "@/hooks/domains/orchestration/use-assistant-input";
import { assistantFailureKey } from "@/hooks/domains/orchestration/use-assistant-actions";
function questionMessages(snapshot: AssistantInputSnapshot): Message[] {
  return (snapshot.input.questions ?? []).map<Message>((question, index) => ({
    id: `${snapshot.attention.id}:${question.id}`,
    session_id: toSessionId(snapshot.input.session_id),
    task_id: toTaskId(snapshot.input.task_id),
    author_type: "agent",
    content: question.prompt,
    type: "clarification_request",
    created_at: snapshot.attention.updated_at,
    metadata: {
      pending_id: snapshot.input.pending_id,
      session_id: snapshot.input.session_id,
      task_id: snapshot.input.task_id,
      question: { ...question, options: question.options ?? [] },
      question_id: question.id,
      question_index: index,
      question_total: snapshot.input.questions?.length,
      status: "pending",
    },
  }));
}
export function AttentionCard({
  binding,
  row,
  onResolved,
  onDismiss,
}: {
  binding: AssistantBinding;
  row: AssistantAttention;
  onResolved: () => void;
  onDismiss: () => void;
}) {
  const { t } = useTranslation();
  const control = useAssistantInput(binding, row, onResolved);
  const scope = useRef<HTMLDivElement>(null);
  const messages = useMemo(
    () => (control.snapshot ? questionMessages(control.snapshot) : []),
    [control.snapshot],
  );
  const input = control.snapshot?.input;
  const permission = input?.permission;
  return (
    <div
      ref={scope}
      className="min-w-0 rounded-md border bg-background p-3 space-y-3"
      data-testid="assistant-attention-card"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-sm font-medium">
          {t(`orchestration:assistantAttention_${row.kind}`)}
        </span>
        <Button variant="ghost" className="cursor-pointer max-md:min-h-11" onClick={onDismiss}>
          {t("common:close")}
        </Button>
      </div>
      {Boolean(control.error) && (
        <div role="alert" className="space-y-2 text-sm">
          <p>
            {t(
              control.snapshot
                ? assistantFailureKey(control.error)
                : "orchestration:assistantUnavailable",
            )}
          </p>
          <Button
            variant="outline"
            className="cursor-pointer max-md:min-h-11"
            onClick={control.refresh}
          >
            {t("orchestration:assistantRefreshInput")}
          </Button>
        </div>
      )}
      {!input && !control.error && <p role="status">{t("common:loading")}</p>}
      {input && input.state !== "pending" && (
        <p role="status">{t(`orchestration:assistantAttentionState_${input.state}`)}</p>
      )}
      {input?.state === "pending" && input.kind === "question" && (
        <ClarificationTransportContext.Provider value={control.transport}>
          <ClarificationInputOverlay
            messages={messages}
            onResolved={onResolved}
            shortcutScopeRef={scope}
            onDismiss={onDismiss}
          />
        </ClarificationTransportContext.Provider>
      )}
      {input?.state === "pending" && permission && (
        <div className="space-y-2">
          <p className="text-sm break-words">{permission.title}</p>
          <pre className="whitespace-pre-wrap break-all text-xs">
            {[
              permission.action.description,
              permission.action.command,
              permission.action.cwd,
              permission.action.path,
              permission.action.destination,
              permission.action.server,
              permission.action.tool,
            ]
              .filter(Boolean)
              .join("\n")}
          </pre>
          <NativePermissionOptions
            options={permission.options}
            isResponding={control.busy}
            onSelect={(option) => void control.resolve({ option_id: option })}
          />
        </div>
      )}
      <Link
        className="inline-flex items-center underline text-sm cursor-pointer max-md:min-h-11"
        href={`/tasks/${encodeURIComponent(row.task_id)}?sessionId=${encodeURIComponent(row.session_id)}`}
      >
        {t("orchestration:assistantOpenTask")}
      </Link>
    </div>
  );
}
