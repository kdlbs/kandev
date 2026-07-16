"use client";

import { useState } from "react";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { useShellCommandOutput } from "@/hooks/domains/session/use-shell-command-output";
import type { ShellCommandOutput, ShellCommandOutputSnapshot } from "@/lib/api/domains/session-api";
import { cn } from "@/lib/utils";
import { isTerminalToolCallStatus, normalizeToolCallStatus } from "@/lib/utils/tool-call-status";
import type { ShellExecOutputSummary } from "../types";

type ShellOutputDisclosureProps = {
  sessionId: string;
  messageId: string;
  messageStatus?: string;
  summary?: ShellExecOutputSummary;
};

function formatOutputSize(summary: ShellExecOutputSummary | undefined) {
  const bytes = (summary?.stdout_bytes ?? 0) + (summary?.stderr_bytes ?? 0);
  if (bytes === 0) return "Output";
  if (bytes < 1024) return `Output · ${bytes} B`;
  return `Output · ${(bytes / 1024).toFixed(bytes < 10_240 ? 1 : 0)} KB`;
}

function OutputTranscript({ output }: { output: ShellCommandOutput }) {
  if (!output.stdout && !output.stderr) return null;
  return (
    <div className="space-y-2" data-testid="tool-execute-output">
      {output.stdout && (
        <pre className="max-h-[200px] overflow-auto rounded bg-muted/30 p-2 whitespace-pre-wrap break-words [overflow-wrap:anywhere] text-xs">
          {output.stdout}
        </pre>
      )}
      {output.stderr && (
        <pre className="max-h-[200px] overflow-auto rounded bg-red-500/10 p-2 whitespace-pre-wrap break-words [overflow-wrap:anywhere] text-xs text-red-600 dark:text-red-400">
          {output.stderr}
        </pre>
      )}
    </div>
  );
}

function ResultDetails({ snapshot }: { snapshot: ShellCommandOutputSnapshot }) {
  const { output, status } = snapshot;
  const normalizedStatus = normalizeToolCallStatus(status);
  if (!isTerminalToolCallStatus(status) && !output.truncated) return null;
  const knownExit = typeof output.exit_code === "number";
  const failed = normalizedStatus === "error" || (knownExit && output.exit_code !== 0);
  const succeeded = normalizedStatus === "complete" && knownExit && output.exit_code === 0;
  let exitClass = "text-muted-foreground";
  if (knownExit && failed) exitClass = "text-red-600 dark:text-red-400";
  if (succeeded) exitClass = "text-green-600 dark:text-green-400";

  return (
    <div className="flex min-w-0 flex-wrap items-center justify-between gap-x-3 gap-y-1 border-t border-border/40 pt-2 text-xs text-muted-foreground">
      {output.truncated && <span>Output truncated</span>}
      {isTerminalToolCallStatus(status) && (
        <span className={cn("font-mono", exitClass)}>
          {knownExit ? `Exit code ${output.exit_code}` : "Exit code unavailable"}
        </span>
      )}
    </div>
  );
}

function DisclosureContent({
  snapshot,
  isLoading,
  error,
  retry,
  messageStatus,
}: {
  snapshot: ShellCommandOutputSnapshot | null;
  isLoading: boolean;
  error: Error | null;
  retry: () => void;
  messageStatus?: string;
}) {
  const hasTranscript = Boolean(snapshot?.output.stdout || snapshot?.output.stderr);
  const emptyLabel =
    normalizeToolCallStatus(snapshot?.status) === "running" &&
    !isTerminalToolCallStatus(messageStatus)
      ? "No command output yet."
      : "No command output.";

  return (
    <div className="min-w-0 space-y-2 border-l-2 border-border/30 pl-3 pt-1">
      {isLoading && !snapshot && (
        <p className="text-xs text-muted-foreground">Loading command output...</p>
      )}
      {error && (
        <div className="flex min-w-0 flex-wrap items-center gap-2 text-xs text-destructive">
          <span>Command output unavailable.</span>
          <Button variant="ghost" size="xs" className="cursor-pointer" onClick={retry}>
            Retry
          </Button>
        </div>
      )}
      {snapshot && !hasTranscript && !error && (
        <p className="text-xs text-muted-foreground">{emptyLabel}</p>
      )}
      {snapshot && <OutputTranscript output={snapshot.output} />}
      {snapshot && <ResultDetails snapshot={snapshot} />}
    </div>
  );
}

export function ShellOutputDisclosure({
  sessionId,
  messageId,
  messageStatus,
  summary,
}: ShellOutputDisclosureProps) {
  const [isOpen, setIsOpen] = useState(false);
  const output = useShellCommandOutput({ sessionId, messageId, isOpen, messageStatus });
  const label = formatOutputSize(summary);

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="-ml-2 h-7 max-w-full cursor-pointer gap-1 px-2 text-muted-foreground"
          aria-label={`${isOpen ? "Hide" : "Show"} command output`}
        >
          {isOpen ? <IconChevronDown aria-hidden /> : <IconChevronRight aria-hidden />}
          <span className="truncate">{label}</span>
          {summary?.truncated && (
            <span className="text-amber-600 dark:text-amber-400">truncated</span>
          )}
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        {isOpen && <DisclosureContent {...output} messageStatus={messageStatus} />}
      </CollapsibleContent>
    </Collapsible>
  );
}
