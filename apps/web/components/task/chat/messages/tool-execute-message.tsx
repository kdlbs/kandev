"use client";

import { memo } from "react";
import { IconCheck, IconX, IconTerminal } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { GridSpinner } from "@/components/grid-spinner";
import { transformPathsInText } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import { ExpandableRow } from "./expandable-row";
import { useExpandState } from "./use-expand-state";

type ShellExecOutput = {
  exit_code?: number;
  stdout?: string;
  stderr?: string;
};

type ShellExecPayload = {
  command?: string;
  work_dir?: string;
  output?: ShellExecOutput;
};

type ToolExecuteMetadata = {
  tool_call_id?: string;
  status?: "pending" | "running" | "complete" | "error";
  normalized?: { shell_exec?: ShellExecPayload };
};

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
    return exitCode === 0 ? (
      <IconCheck className="h-3.5 w-3.5 text-green-500" />
    ) : (
      <IconX className="h-3.5 w-3.5 text-red-500" />
    );
  }
  if (status === "error") return <IconX className="h-3.5 w-3.5 text-red-500" />;
  if (status === "running") return <GridSpinner className="text-muted-foreground" />;
  return null;
}

type ExecuteOutputProps = {
  displayCommand: string;
  displayWorkDir: string | null;
  workDir: string | undefined;
  output: ShellExecOutput | undefined;
};

function ExecuteOutputContent({
  displayCommand,
  displayWorkDir,
  workDir,
  output,
}: ExecuteOutputProps) {
  return (
    <div className="pl-4 border-l-2 border-border/30 space-y-2">
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
      {output?.stdout && (
        <pre className="text-xs bg-muted/30 rounded p-2 overflow-x-auto whitespace-pre-wrap max-h-[200px]">
          {output.stdout}
        </pre>
      )}
      {output?.stderr && (
        <pre className="text-xs bg-red-500/10 text-red-600 dark:text-red-400 rounded max-h-[200px] p-2 overflow-x-auto whitespace-pre-wrap">
          {output.stderr}
        </pre>
      )}
    </div>
  );
}

function isExecuteSuccess(
  status: ToolExecuteMetadata["status"],
  output: ShellExecOutput | undefined,
): boolean {
  if (status !== "complete") return false;
  return output?.exit_code === 0 || output?.exit_code === undefined;
}

function parseExecuteMetadata(comment: Message) {
  const metadata = comment.metadata as ToolExecuteMetadata | undefined;
  const status = metadata?.status;
  const shellExec = metadata?.normalized?.shell_exec;
  const output = shellExec?.output;
  const workDir = shellExec?.work_dir;
  const isSuccess = isExecuteSuccess(status, output);
  return { status, output, workDir, isSuccess };
}

function CommandHeader({ displayCommand }: { displayCommand: string }) {
  const className = "font-mono text-xs text-muted-foreground truncate min-w-0 flex-1 text-left";
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className={className} tabIndex={0}>
          {displayCommand}
        </span>
      </TooltipTrigger>
      <TooltipContent
        side="bottom"
        align="start"
        className="max-w-[min(90vw,640px)] whitespace-pre-wrap break-all font-mono text-xs"
      >
        {displayCommand}
      </TooltipContent>
    </Tooltip>
  );
}

export const ToolExecuteMessage = memo(function ToolExecuteMessage({
  comment,
  worktreePath,
}: ToolExecuteMessageProps) {
  const { status, output, workDir, isSuccess } = parseExecuteMetadata(comment);
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
          {!isSuccess && (
            <span className="shrink-0">
              <ExecuteStatusIcon status={status} exitCode={output?.exit_code} />
            </span>
          )}
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
      />
    </ExpandableRow>
  );
});
