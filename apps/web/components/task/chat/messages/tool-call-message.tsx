"use client";

import { useCallback, useState, memo } from "react";
import {
  IconCheck,
  IconCode,
  IconEdit,
  IconEye,
  IconFile,
  IconSearch,
  IconTerminal2,
  IconX,
} from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { cn, transformPathsInText } from "@/lib/utils";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { Message } from "@/lib/types/http";
import type { ToolCallMetadata } from "@/components/task/chat/types";
import { PermissionActionRow } from "./permission-action-row";
import { ExpandableRow } from "./expandable-row";
import { useExpandState } from "./use-expand-state";

type PermissionOption = {
  option_id: string;
  name: string;
  kind: string;
};

type PermissionRequestMetadata = {
  pending_id: string;
  tool_call_id: string;
  options: PermissionOption[];
  action_type: string;
  action_details: { command?: string; path?: string; cwd?: string };
  status?: "pending" | "approved" | "rejected";
};

const TOOL_ICON_RULES: Array<{ keywords: string[]; Icon: typeof IconCode }> = [
  { keywords: ["edit", "replace", "write", "save"], Icon: IconEdit },
  { keywords: ["view", "read"], Icon: IconEye },
  { keywords: ["search", "find", "retrieval"], Icon: IconSearch },
  { keywords: ["terminal", "exec", "execute", "launch", "process"], Icon: IconTerminal2 },
  { keywords: ["delete", "move", "file", "create"], Icon: IconFile },
];

function getToolIcon(toolName: string | undefined, className: string) {
  const name = toolName?.toLowerCase() ?? "";
  const match = TOOL_ICON_RULES.find(({ keywords }) =>
    keywords.some((kw) => name === kw || name.includes(kw)),
  );
  const Icon = match?.Icon ?? IconCode;
  return <Icon className={className} />;
}

type ToolCallMessageProps = {
  comment: Message;
  permissionMessage?: Message;
  worktreePath?: string;
};

function getToolCallStatusIcon(status: string | undefined, permissionStatus: string | undefined) {
  if (permissionStatus === "approved") return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
  if (permissionStatus === "rejected") return <IconX className="h-3.5 w-3.5 text-red-500" />;
  if (status === "complete") return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
  if (status === "error") return <IconX className="h-3.5 w-3.5 text-red-500" />;
  if (status === "running") return <GridSpinner className="text-muted-foreground" />;
  return null;
}

function formatToolOutput(value: unknown): { content: string; isJson: boolean } {
  if (typeof value === "string") {
    try {
      const parsed = JSON.parse(value);
      return { content: JSON.stringify(parsed, null, 2), isJson: true };
    } catch {
      return { content: value, isJson: false };
    }
  }
  return { content: JSON.stringify(value, null, 2), isJson: true };
}

type ToolCallExpandedContentProps = {
  formattedOutput: { content: string; isJson: boolean } | null;
  isHttpError: boolean | undefined;
  isPermissionPending: boolean;
  onApprove: () => void;
  onReject: () => void;
  isResponding: boolean;
};

function ToolCallExpandedContent({
  formattedOutput,
  isHttpError,
  isPermissionPending,
  onApprove,
  onReject,
  isResponding,
}: ToolCallExpandedContentProps) {
  return (
    <div className="pl-4 border-l-2 border-border/30 space-y-2">
      {formattedOutput && (
        <pre
          className={cn(
            "text-xs rounded p-2 overflow-x-auto whitespace-pre-wrap max-h-[200px] overflow-y-auto",
            formattedOutput.isJson ? "bg-muted/30 font-mono text-[11px]" : "bg-muted/30",
            isHttpError && "text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-950/30",
          )}
        >
          {formattedOutput.content}
        </pre>
      )}
      {isPermissionPending && (
        <PermissionActionRow
          onApprove={onApprove}
          onReject={onReject}
          isResponding={isResponding}
        />
      )}
    </div>
  );
}

function hasToolOutput(output: unknown): boolean {
  if (!output) return false;
  if (typeof output === "string") return output.length > 0;
  return Object.keys(output as object).length > 0;
}

function parsePermission(permissionMessage: Message | undefined) {
  const permissionMetadata = permissionMessage?.metadata as PermissionRequestMetadata | undefined;
  const permissionStatus = permissionMetadata?.status;
  const isPermissionPending =
    !!permissionMessage && permissionStatus !== "approved" && permissionStatus !== "rejected";
  return { permissionMetadata, permissionStatus, isPermissionPending };
}

function parseToolCallOutput(metadata: ToolCallMetadata | undefined) {
  const normalizedGeneric = metadata?.normalized?.generic;
  const normalizedHttpRequest = metadata?.normalized?.http_request;
  const output = normalizedHttpRequest?.response ?? normalizedGeneric?.output ?? metadata?.result;
  const isHttpError = normalizedHttpRequest?.is_error;
  return { output, isHttpError };
}

function parseToolCallMetadata(comment: Message, permissionMessage: Message | undefined) {
  const metadata = comment.metadata as ToolCallMetadata | undefined;
  const toolName = metadata?.tool_name ?? "";
  const status = metadata?.status;
  const { output, isHttpError } = parseToolCallOutput(metadata);
  const { permissionMetadata, permissionStatus, isPermissionPending } =
    parsePermission(permissionMessage);
  const hasOutput = hasToolOutput(output);
  const hasExpandableContent = hasOutput || isPermissionPending;
  const isSuccess = status === "complete" && !permissionStatus;
  return {
    toolName,
    status,
    output,
    isHttpError,
    permissionMetadata,
    permissionStatus,
    isPermissionPending,
    hasOutput,
    hasExpandableContent,
    isSuccess,
  };
}

type UsePermissionHandlersParams = {
  permissionMetadata: PermissionRequestMetadata | undefined;
  permissionMessage: Message | undefined;
};

function usePermissionResponseHandlers({
  permissionMetadata,
  permissionMessage,
}: UsePermissionHandlersParams) {
  const [isResponding, setIsResponding] = useState(false);

  const handleRespond = useCallback(
    async (optionId: string, cancelled: boolean = false) => {
      if (!permissionMetadata || !permissionMessage) return;
      const client = getWebSocketClient();
      if (!client) {
        console.error("WebSocket client not available");
        return;
      }
      setIsResponding(true);
      try {
        await client.request("permission.respond", {
          session_id: permissionMessage.session_id,
          pending_id: permissionMetadata.pending_id,
          option_id: cancelled ? undefined : optionId,
          cancelled,
        });
      } catch (error) {
        console.error("Failed to respond to permission request:", error);
      } finally {
        setIsResponding(false);
      }
    },
    [permissionMessage, permissionMetadata],
  );

  const handleApprove = useCallback(() => {
    const allowOption = permissionMetadata?.options.find(
      (opt) => opt.kind === "allow_once" || opt.kind === "allow_always",
    );
    if (allowOption) handleRespond(allowOption.option_id);
  }, [permissionMetadata, handleRespond]);

  const handleReject = useCallback(() => {
    const rejectOption = permissionMetadata?.options.find(
      (opt) => opt.kind === "reject_once" || opt.kind === "reject_always",
    );
    if (rejectOption) {
      handleRespond(rejectOption.option_id);
    } else {
      handleRespond("", true);
    }
  }, [permissionMetadata, handleRespond]);

  return { isResponding, handleApprove, handleReject };
}

export const ToolCallMessage = memo(function ToolCallMessage({
  comment,
  permissionMessage,
  worktreePath,
}: ToolCallMessageProps) {
  const {
    toolName,
    status,
    output,
    isHttpError,
    permissionMetadata,
    permissionStatus,
    isPermissionPending,
    hasOutput,
    hasExpandableContent,
    isSuccess,
  } = parseToolCallMetadata(comment, permissionMessage);
  const { isResponding, handleApprove, handleReject } = usePermissionResponseHandlers({
    permissionMetadata,
    permissionMessage,
  });
  const autoExpanded = status === "running" || isPermissionPending;
  const { isExpanded, handleToggle } = useExpandState(status, autoExpanded);

  const metadata = comment.metadata as ToolCallMetadata | undefined;
  const rawTitle = metadata?.title ?? comment.content ?? "Tool call";
  const title = transformPathsInText(rawTitle, worktreePath);

  const formattedOutput = hasOutput ? formatToolOutput(output) : null;

  return (
    <ExpandableRow
      icon={getToolIcon(
        toolName,
        cn(
          "h-4 w-4",
          isPermissionPending ? "text-amber-600 dark:text-amber-400" : "text-muted-foreground",
        ),
      )}
      header={
        <div className="flex items-center gap-2 text-xs">
          <span
            className={cn(
              "inline-flex items-center gap-1.5",
              isPermissionPending && "text-amber-600 dark:text-amber-400",
            )}
          >
            <span className="font-mono text-xs text-muted-foreground">{title}</span>
            {!isSuccess && getToolCallStatusIcon(status, permissionStatus)}
          </span>
        </div>
      }
      hasExpandableContent={!!hasExpandableContent}
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      <ToolCallExpandedContent
        formattedOutput={formattedOutput}
        isHttpError={isHttpError}
        isPermissionPending={isPermissionPending}
        onApprove={handleApprove}
        onReject={handleReject}
        isResponding={isResponding}
      />
    </ExpandableRow>
  );
});
