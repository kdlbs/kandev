"use client";

import { memo } from "react";
import { IconCheck, IconX, IconTerminal } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { transformPathsInText } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import { ExpandableRow } from "./expandable-row";
import { useExpandState } from "./use-expand-state";
import type { ShellExecOutput, ToolCallMetadata } from "../types";

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
  if (status === "complete") {
    if (exitCode === 0) {
      return (
        <span className="shrink-0" aria-label="Command succeeded">
          <IconCheck aria-hidden className="h-3.5 w-3.5 text-green-500" />
        </span>
      );
    }
    if (typeof exitCode === "number") {
      return (
        <span className="shrink-0" aria-label="Command failed">
          <IconX aria-hidden className="h-3.5 w-3.5 text-red-500" />
        </span>
      );
    }
    return null;
  }
  if (status === "error") {
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

type ExecuteOutputProps = {
  displayCommand: string;
  displayWorkDir: string | null;
  workDir: string | undefined;
  output: ShellExecOutput | undefined;
  status: ToolCallMetadata["status"];
};

function TerminalOutput({ output }: { output: ShellExecOutput | undefined }) {
  if (!output?.stdout && !output?.stderr) return null;
  return (
    <div className="space-y-2" data-testid="tool-execute-output">
      {output.stdout && (
        <pre className="text-xs bg-muted/30 rounded p-2 overflow-x-auto whitespace-pre-wrap break-words max-h-[200px]">
          {output.stdout}
        </pre>
      )}
      {output.stderr && (
        <pre className="text-xs bg-red-500/10 text-red-600 dark:text-red-400 rounded max-h-[200px] p-2 overflow-x-auto whitespace-pre-wrap break-words">
          {output.stderr}
        </pre>
      )}
    </div>
  );
}

function ExecuteResultDetails({
  output,
  status,
}: {
  output: ShellExecOutput | undefined;
  status: ToolCallMetadata["status"];
}) {
  const isTerminal = status === "complete" || status === "error" || status === "cancelled";
  if (!isTerminal && !output?.truncated) return null;
  const exitLabel =
    typeof output?.exit_code === "number"
      ? `Exit code ${output.exit_code}`
      : "Exit code unavailable";

  return (
    <div
      className="flex min-w-0 flex-wrap items-center justify-between gap-x-3 gap-y-1 border-t border-border/40 pt-2 text-xs text-muted-foreground"
      data-testid="tool-execute-result-details"
    >
      {output?.truncated && <span>Output truncated</span>}
      {isTerminal && <span className="font-mono">{exitLabel}</span>}
    </div>
  );
}

function ExecuteOutputContent({
  displayCommand,
  displayWorkDir,
  workDir,
  output,
  status,
}: ExecuteOutputProps) {
  return (
    <div className="min-w-0 pl-4 border-l-2 border-border/30 space-y-2">
      <pre className="text-xs bg-muted/30 rounded p-2 whitespace-pre-wrap break-all font-mono">
        {displayCommand}
      </pre>
      {displayWorkDir && (
        <div className="text-xs text-muted-foreground">
          <span className="opacity-60">cwd:</span>{" "}
          <span className="font-mono" title={workDir}>
            {displayWorkDir}
          </span>
        </div>
      )}
      <TerminalOutput output={output} />
      <ExecuteResultDetails output={output} status={status} />
    </div>
  );
}

function parseExecuteMetadata(comment: Message) {
  const metadata = comment.metadata as ToolCallMetadata | undefined;
  const status = metadata?.status === "in_progress" ? "running" : metadata?.status;
  const shellExec = metadata?.normalized?.shell_exec;
  const output = shellExec?.output;
  const workDir = shellExec?.work_dir;
  return { status, output, workDir };
}

function CommandHeader({ displayCommand }: { displayCommand: string }) {
  return (
    <span
      className="font-mono text-xs text-muted-foreground truncate min-w-0 flex-1 text-left"
      data-testid="tool-execute-command"
    >
      {displayCommand}
    </span>
  );
}

export const ToolExecuteMessage = memo(function ToolExecuteMessage({
  comment,
  worktreePath,
}: ToolExecuteMessageProps) {
  const { status, output, workDir } = parseExecuteMetadata(comment);
  const autoExpanded = status === "running";
  const { isExpanded, handleToggle } = useExpandState(status, autoExpanded);
  const displayCommand = transformPathsInText(comment.content, worktreePath);
  const displayWorkDir = workDir ? transformPathsInText(workDir, worktreePath) : null;

  return (
    <ExpandableRow
      icon={<IconTerminal className="h-4 w-4 text-muted-foreground" />}
      header={
        <div className="flex items-center gap-2 text-xs min-w-0">
          <CommandHeader displayCommand={displayCommand} />
          <ExecuteStatusIcon status={status} exitCode={output?.exit_code} />
        </div>
      }
      hasExpandableContent
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      <ExecuteOutputContent
        displayCommand={displayCommand}
        displayWorkDir={displayWorkDir}
        workDir={workDir}
        output={output}
        status={status}
      />
    </ExpandableRow>
  );
});
