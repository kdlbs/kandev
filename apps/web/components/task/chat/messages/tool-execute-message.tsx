"use client";

import { memo } from "react";
import { IconCheck, IconTerminal, IconX } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { transformPathsInText } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import { ShellOutputDisclosure } from "./shell-output-disclosure";
import { normalizeToolCallStatus } from "./tool-status";
import type { ToolCallMetadata } from "../types";

type ToolExecuteMessageProps = {
  comment: Message;
  worktreePath?: string;
};

function ExecuteStatusIcon({
  status,
  exitCode,
}: {
  status: string | undefined;
  exitCode: number | undefined;
}) {
  if (status === "complete" && exitCode === 0) {
    return (
      <span className="shrink-0" aria-label="Command succeeded">
        <IconCheck aria-hidden className="h-3.5 w-3.5 text-green-500" />
      </span>
    );
  }
  if (
    status === "error" ||
    (status === "complete" && typeof exitCode === "number" && exitCode !== 0)
  ) {
    return (
      <span className="shrink-0" aria-label="Command failed">
        <IconX aria-hidden className="h-3.5 w-3.5 text-red-500" />
      </span>
    );
  }
  if (status === "running") {
    return (
      <span className="shrink-0" aria-label="Command running">
        <GridSpinner className="text-muted-foreground" />
      </span>
    );
  }
  return null;
}

function parseExecuteMetadata(comment: Message) {
  const metadata = comment.metadata as ToolCallMetadata | undefined;
  const status = normalizeToolCallStatus(metadata?.status);
  const shellExec = metadata?.normalized?.shell_exec;
  return { status, shellExec };
}

export const ToolExecuteMessage = memo(function ToolExecuteMessage({
  comment,
  worktreePath,
}: ToolExecuteMessageProps) {
  const { status, shellExec } = parseExecuteMetadata(comment);
  const command = shellExec?.command || comment.content;
  const displayCommand = transformPathsInText(command, worktreePath);
  const workDir = shellExec?.work_dir;
  const displayWorkDir = workDir ? transformPathsInText(workDir, worktreePath) : null;

  return (
    <div className="flex w-full min-w-0 items-start gap-3 px-2 py-1 -mx-2">
      <IconTerminal className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" aria-hidden />
      <div className="min-w-0 flex-1 space-y-1.5">
        <div className="flex min-w-0 items-start gap-2">
          <pre
            className="min-w-0 flex-1 whitespace-pre-wrap break-words [overflow-wrap:anywhere] text-left font-mono text-xs text-muted-foreground"
            data-testid="tool-execute-command"
          >
            {displayCommand}
          </pre>
          <ExecuteStatusIcon status={status} exitCode={shellExec?.output?.exit_code} />
        </div>
        {displayWorkDir && (
          <div className="min-w-0 whitespace-pre-wrap break-words [overflow-wrap:anywhere] text-xs text-muted-foreground">
            <span className="opacity-60">cwd:</span>{" "}
            <span className="font-mono" title={workDir}>
              {displayWorkDir}
            </span>
          </div>
        )}
        <ShellOutputDisclosure
          sessionId={comment.session_id}
          messageId={comment.id}
          messageStatus={status}
          summary={shellExec?.output}
        />
      </div>
    </div>
  );
});
