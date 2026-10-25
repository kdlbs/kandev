import { useTranslation } from "react-i18next";
import { IconTool } from "@tabler/icons-react";
import type { Message } from "@/lib/types/http";
import { payloadRetentionMarker } from "@/lib/utils/tool-payload-retention";
import { normalizeToolCallStatus } from "@/lib/utils/tool-call-status";
import { formatDateTime } from "@/lib/i18n/formats";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import type { ToolCallMetadata } from "../types";

export function ToolPayloadRemovedNotice({ metadata }: { metadata: unknown }) {
  const { t } = useTranslation();
  const marker = payloadRetentionMarker(metadata);
  const date = marker?.removed_at;
  return (
    <p className="break-words text-xs text-muted-foreground" data-testid="tool-payload-removed">
      {date && parseTurnTimestamp(date) !== null
        ? t("task:toolPayloadRemovedAt", { date: formatDateTime(date) })
        : t("task:toolPayloadRemoved")}
    </p>
  );
}

function retainedSummary(metadata: ToolCallMetadata | undefined) {
  const normalized = metadata?.normalized;
  if (!normalized) return undefined;
  return (
    normalized.read_file?.file_path ||
    normalized.modify_file?.file_path ||
    normalized.code_search?.query ||
    normalized.http_request?.url ||
    normalized.generic?.name
  );
}

export function ToolPayloadRemovedMessage({ comment }: { comment: Message }) {
  const { t } = useTranslation();
  const metadata = comment.metadata as ToolCallMetadata | undefined;
  const status = normalizeToolCallStatus(metadata?.status);
  const summary = retainedSummary(metadata);
  const title =
    typeof comment.metadata?.title === "string" ? comment.metadata.title : comment.content;
  const statuses = {
    complete: "task:completed",
    error: "task:error",
    cancelled: "task:cancelled",
    running: "task:running",
    pending: "system:toolPayload.pending",
  } as const;
  return (
    <div className="flex min-w-0 gap-3 py-1">
      <IconTool aria-hidden className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex min-w-0 flex-wrap items-start justify-between gap-2 text-xs">
          <span className="break-words [overflow-wrap:anywhere]">{title}</span>
          {status && <span>{t(statuses[status])}</span>}
        </div>
        <ToolPayloadRemovedNotice metadata={metadata} />
        {summary && (
          <p className="break-words text-xs text-muted-foreground [overflow-wrap:anywhere]">
            {summary}
          </p>
        )}
      </div>
    </div>
  );
}
