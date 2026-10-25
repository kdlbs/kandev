"use client";

import { useState, useCallback, memo } from "react";
import { IconCheck, IconChevronDown, IconChevronRight, IconX } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import type { SubagentTaskPayload, ToolCallMetadata } from "@/components/task/chat/types";
import { SubagentMetaRow } from "@/components/task/chat/messages/subagent-meta-row";
import { isTerminalToolCallStatus } from "@/lib/utils/tool-call-status";
import { normalizeToolCallStatus } from "./tool-status";
import { useTranslation } from "react-i18next";

type ToolSubagentMessageProps = {
  comment: Message;
  childMessages: Message[];
  isContainingTurnActive?: boolean;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  renderChild: (message: Message) => React.ReactNode;
};

// Custom comparison function that ignores renderChild (it's always recreated but referentially stable in behavior)
function arePropsEqual(
  prevProps: ToolSubagentMessageProps,
  nextProps: ToolSubagentMessageProps,
): boolean {
  return (
    prevProps.comment === nextProps.comment &&
    prevProps.childMessages === nextProps.childMessages &&
    prevProps.isContainingTurnActive === nextProps.isContainingTurnActive &&
    prevProps.worktreePath === nextProps.worktreePath &&
    prevProps.onOpenFile === nextProps.onOpenFile
  );
}

const LIVE_SUBAGENT_PAYLOAD_STATUSES = new Set(["pendingInit", "started", "async_launched"]);

// Mirrors backend subagentStatusTerminal: Codex session statuses that mean the
// nested task has already settled, even while the parent turn is still active.
const TERMINAL_SUBAGENT_PAYLOAD_STATUSES = new Set([
  "completed",
  "failed",
  "cancelled",
  "errored",
  "interrupted",
  "shutdown",
  "notFound",
]);

function isSubagentPayloadTerminal(status: string | undefined): boolean {
  if (!status) return false;
  return TERMINAL_SUBAGENT_PAYLOAD_STATUSES.has(status) || isTerminalToolCallStatus(status);
}

function isSubagentPayloadLive(status: string | undefined): boolean {
  if (!status) return false;
  if (LIVE_SUBAGENT_PAYLOAD_STATUSES.has(status)) return true;
  return !isSubagentPayloadTerminal(status);
}

type SubagentTerminalStatus = "complete" | "error" | "cancelled";

function normalizeSubagentTerminalStatus(
  status: string | undefined,
): SubagentTerminalStatus | undefined {
  if (status === "errored" || status === "notFound") return "error";
  if (status === "interrupted" || status === "shutdown") return "cancelled";

  const normalized = normalizeToolCallStatus(status);
  if (normalized === "complete" || normalized === "error" || normalized === "cancelled") {
    return normalized;
  }
  return undefined;
}

function resolveSubagentTerminalStatus(
  metadataStatus: ToolCallMetadata["status"] | undefined,
  payloadStatus: string | undefined,
): SubagentTerminalStatus {
  const payloadTerminal = normalizeSubagentTerminalStatus(payloadStatus);
  const metadataTerminal = normalizeSubagentTerminalStatus(metadataStatus);
  if (payloadTerminal && payloadTerminal !== "complete") return payloadTerminal;
  if (metadataTerminal && metadataTerminal !== "complete") return metadataTerminal;
  return payloadTerminal ?? metadataTerminal ?? "complete";
}

export function isSubagentEffectivelyActive(
  metadata: ToolCallMetadata | undefined,
  isContainingTurnActive: boolean,
): boolean {
  const payloadStatus = metadata?.normalized?.subagent_task?.status;
  if (payloadStatus && isSubagentPayloadTerminal(payloadStatus)) return false;

  const status = metadata?.status;
  if (status === "running") return true;

  const normalized = normalizeToolCallStatus(status);
  if (normalized === "error" || normalized === "cancelled") return false;

  if (!isContainingTurnActive) return false;

  if (status === "in_progress" || isSubagentPayloadLive(payloadStatus)) return true;
  return false;
}

// The result is what the subagent was dispatched to produce, so it is shown
// whether or not the subagent also streamed child tool calls. The prompt is a
// fallback for subagents that reported neither.
function deriveSubagentBody(
  childCount: number,
  subagentTask: SubagentTaskPayload | undefined,
): { resultText?: string; prompt?: string } {
  if (subagentTask?.result_text) return { resultText: subagentTask.result_text };
  if (childCount > 0) return {};
  if (subagentTask?.prompt) return { prompt: subagentTask.prompt };
  return {};
}

/** The collapsed card carries one line of the result — the first non-empty one,
 *  which is where agents put the verdict. Everything else waits for expand. */
export function firstResultLine(resultText: string | undefined): string {
  if (!resultText) return "";
  for (const line of resultText.split("\n")) {
    const trimmed = line.trim();
    if (trimmed) return trimmed;
  }
  return "";
}

function deriveSubagentDisplay(
  metadata: ToolCallMetadata | undefined,
  childCount: number,
  isContainingTurnActive: boolean,
) {
  const subagentTask = metadata?.normalized?.subagent_task;
  const isActive = isSubagentEffectivelyActive(metadata, isContainingTurnActive);
  const { resultText, prompt } = deriveSubagentBody(childCount, subagentTask);
  return {
    subagentTask,
    isActive,
    resultText,
    prompt,
    hasExpandableContent: childCount > 0 || Boolean(resultText) || Boolean(prompt),
  };
}

/** The type chip already names the subagent, so a description that opens by
 *  restating it ("test-supervisor" + "Test-supervisor review of new invariant
 *  tests") spends the line it truncates in on a word already on screen. The
 *  match ignores case and the separators agents vary on, and only strips at a
 *  word boundary — "code-reviewer" must not eat the "code-review" in
 *  "code-review of the closure diff". Returns "" when the description is
 *  nothing but the type. */
export function stripSubagentTypePrefix(description: string, subagentType: string): string {
  const normalize = (value: string) => value.toLowerCase().replace(/[\s\-_]+/g, "");
  const target = normalize(subagentType);
  if (!target) return description;
  let consumed = "";
  for (let i = 0; i < description.length; i++) {
    consumed += normalize(description[i]);
    if (consumed.length < target.length) continue;
    if (consumed !== target) return description;
    // Only whitespace or a colon separates a type from its description. `.`
    // and `,` are not boundaries: type "test" would otherwise eat the filename
    // in "test.ts regression suite" and leave "ts regression suite".
    const rest = description.slice(i + 1);
    if (rest !== "" && !/^[\s:]/.test(rest)) return description;
    return rest.replace(/^[\s:]+/, "");
  }
  return description;
}

type SubagentHeaderProps = {
  isExpanded: boolean;
  subagentType: string;
  description: string;
  isActive: boolean;
  status?: ToolCallMetadata["status"];
  payloadStatus?: string;
  childCount: number;
  hasExpandableContent: boolean;
  onToggle: () => void;
};

function SubagentStatusIcon({
  isActive,
  status,
  payloadStatus,
}: {
  isActive: boolean;
  status?: ToolCallMetadata["status"];
  payloadStatus?: string;
}) {
  const { t } = useTranslation();
  if (isActive) {
    const workingLabel = t("task:statusWorking");
    return (
      <span className="inline-flex items-center gap-1.5 shrink-0" title={workingLabel}>
        <span className="text-xs text-muted-foreground italic">{t("task:working")}</span>
        <GridSpinner className="text-muted-foreground shrink-0" />
      </span>
    );
  }
  const normalized = resolveSubagentTerminalStatus(status, payloadStatus);
  if (normalized === "error" || normalized === "cancelled") {
    const label = normalized === "cancelled" ? t("task:statusCancelled") : t("task:failed");
    return (
      <span className="shrink-0" title={label} aria-label={label}>
        <IconX aria-hidden className="h-3.5 w-3.5 text-red-500" />
      </span>
    );
  }
  const completedLabel = t("task:statusCompleted");
  return (
    <span className="shrink-0" title={completedLabel} aria-label={completedLabel}>
      <IconCheck aria-hidden className="h-3.5 w-3.5 text-muted-foreground" />
    </span>
  );
}

function SubagentHeader({
  isExpanded,
  subagentType,
  description,
  isActive,
  status,
  payloadStatus,
  childCount,
  hasExpandableContent,
  onToggle,
}: SubagentHeaderProps) {
  const { t } = useTranslation();
  const shownDescription = stripSubagentTypePrefix(description, subagentType);
  const showDescription = shownDescription !== "";
  const content = (
    <>
      {hasExpandableContent &&
        (isExpanded ? (
          <IconChevronDown
            data-testid="subagent-chevron"
            className="h-3.5 w-3.5 text-muted-foreground/60 flex-shrink-0"
          />
        ) : (
          <IconChevronRight
            data-testid="subagent-chevron"
            className="h-3.5 w-3.5 text-muted-foreground/60 flex-shrink-0"
          />
        ))}
      <span
        data-testid="subagent-type"
        className="bg-muted text-muted-foreground text-[10px] px-1.5 rounded font-medium uppercase tracking-wide whitespace-nowrap flex-shrink-0"
      >
        {subagentType}
      </span>
      {showDescription && (
        <span
          data-testid="subagent-description"
          title={description}
          className="font-mono text-xs truncate text-muted-foreground min-w-0"
        >
          {shownDescription}
        </span>
      )}
      <SubagentStatusIcon isActive={isActive} status={status} payloadStatus={payloadStatus} />
      {childCount > 0 && (
        <span
          data-testid="subagent-child-count"
          className="text-muted-foreground/60 text-xs px-1.5 rounded min-w-[20px] text-center font-mono whitespace-nowrap"
        >
          {t("task:toolCallCount", { count: childCount })}
        </span>
      )}
    </>
  );
  if (!hasExpandableContent) {
    return (
      <div
        data-testid="subagent-header"
        className="flex items-center gap-2 w-full text-left px-2 py-1.5 -mx-2 rounded"
      >
        {content}
      </div>
    );
  }
  return (
    <button
      type="button"
      aria-expanded={isExpanded}
      onClick={onToggle}
      className={cn(
        "flex min-h-11 items-center gap-2 w-full text-left px-2 py-1.5 -mx-2 rounded sm:min-h-0",
        "hover:bg-muted/30 transition-colors cursor-pointer",
      )}
    >
      {content}
    </button>
  );
}

const NESTED_BORDER = "ml-2 pl-4 border-l-2 border-border/30 mt-1";

type SubagentContentProps = {
  isExpanded: boolean;
  childMessages: Message[];
  prompt?: string;
  resultText?: string;
  renderChild: (message: Message) => React.ReactNode;
};

function SubagentContent({
  isExpanded,
  childMessages,
  prompt,
  resultText,
  renderChild,
}: SubagentContentProps) {
  if (!isExpanded) return null;
  if (childMessages.length > 0) {
    return (
      <div className={cn(NESTED_BORDER, "space-y-2")}>
        {resultText && (
          <p
            data-testid="subagent-result-text"
            className="text-xs text-foreground/80 whitespace-pre-wrap break-words"
          >
            {resultText}
          </p>
        )}
        {childMessages.map((child) => (
          <div key={child.id}>{renderChild(child)}</div>
        ))}
      </div>
    );
  }
  if (resultText) {
    return (
      <div className={NESTED_BORDER}>
        <p
          data-testid="subagent-result-text"
          className="text-xs text-foreground/80 whitespace-pre-wrap break-words"
        >
          {resultText}
        </p>
      </div>
    );
  }
  if (prompt) {
    return (
      <div className={NESTED_BORDER}>
        <p className="text-xs text-muted-foreground whitespace-pre-wrap break-words">{prompt}</p>
      </div>
    );
  }
  return null;
}

export const ToolSubagentMessage = memo(function ToolSubagentMessage({
  comment,
  childMessages,
  isContainingTurnActive = false,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  worktreePath,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  onOpenFile,
  renderChild,
}: ToolSubagentMessageProps) {
  const { t } = useTranslation();
  const metadata = comment.metadata as ToolCallMetadata | undefined;
  const childCount = childMessages.length;
  const { subagentTask, isActive, resultText, prompt, hasExpandableContent } =
    deriveSubagentDisplay(metadata, childCount, isContainingTurnActive);
  const resultSummary = firstResultLine(resultText);
  const description = subagentTask?.description || comment.content || t("task:subagent");
  const subagentType = subagentTask?.subagent_type || t("common:task");

  // Track manual override state - null means "use auto behavior"
  const [manualExpandState, setManualExpandState] = useState<boolean | null>(null);

  // Auto behavior: expand only while the subagent is active. On completion the
  // card auto-collapses to its header + metadata row, matching subagents that
  // stream child tool calls; the result text is one click away. Previously
  // auto-expand was also keyed on result_text, which left silent (Auggie-style)
  // subagents — the ones with no child messages — permanently expanded because
  // result_text never clears.
  const autoExpanded = isActive;

  // Derive expanded state: manual override takes precedence, otherwise use auto
  const isExpanded = hasExpandableContent && (manualExpandState ?? autoExpanded);

  const handleToggle = useCallback(() => {
    setManualExpandState((prev) => !(prev ?? autoExpanded));
  }, [autoExpanded]);

  return (
    <div className="w-full" data-testid="subagent-card">
      <SubagentHeader
        isExpanded={isExpanded}
        subagentType={subagentType}
        description={description}
        isActive={isActive}
        status={metadata?.status}
        payloadStatus={subagentTask?.status}
        childCount={childCount}
        hasExpandableContent={hasExpandableContent}
        onToggle={handleToggle}
      />
      {!isActive && !isExpanded && resultSummary && (
        <p
          data-testid="subagent-result-summary"
          title={resultSummary}
          className="ml-2 pl-4 text-xs text-foreground/80 truncate"
        >
          {resultSummary}
        </p>
      )}
      {!isActive && <SubagentMetaRow subagentTask={subagentTask} childCount={childCount} />}
      <SubagentContent
        isExpanded={isExpanded}
        childMessages={childMessages}
        prompt={prompt}
        resultText={resultText}
        renderChild={renderChild}
      />
    </div>
  );
}, arePropsEqual);
