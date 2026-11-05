"use client";

import { useState, useRef, useEffect, useMemo } from "react";
import {
  IconPlayerStop,
  IconPlayerPause,
  IconSearch,
  IconSend,
  IconTrash,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { useTranslation } from "react-i18next";
import { PanelRoot } from "@/components/task/panel-primitives";
import { useAppStore } from "@/components/state-provider";
import { useBackgroundWork } from "@/hooks/domains/session/use-background-work";
import { useClearedOutput } from "@/hooks/domains/session/use-cleared-background-output";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import { canRunWorkloadAction, hasReadableOutput } from "@/lib/background-work/capabilities";
import { BackgroundWorkOverview } from "./background-work-overview";
import {
  workloadExitStatusKey,
  workloadKindLabelKey,
  workloadOutputUnavailableKey,
} from "./presentation";
import { WorkloadStateBadge } from "./workload-state-badge";

export type BackgroundWorkPanelProps = {
  panelId: string;
  params?: Record<string, unknown>;
};

export function BackgroundWorkPanel({ params }: BackgroundWorkPanelProps) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionId =
    typeof params?.sessionId === "string" && params.sessionId ? params.sessionId : activeSessionId;
  const workId = typeof params?.workId === "string" ? params.workId : undefined;
  const { workloads, isLoading, isInitialLoading, loadError, executeAction, fetchWorkloads } =
    useBackgroundWork(sessionId);

  const selectedWorkload = useMemo(() => {
    if (!workId) return null;
    return workloads.find((w) => w.work_id === workId) ?? null;
  }, [workloads, workId]);

  if (selectedWorkload) {
    return (
      <WorkloadDetailView
        sessionId={sessionId ?? ""}
        workload={selectedWorkload}
        onAction={executeAction}
      />
    );
  }

  return (
    <PanelRoot data-testid="background-work-overview-panel">
      <BackgroundWorkOverview
        workloads={workloads}
        isLoading={isLoading || isInitialLoading}
        loadError={loadError}
        onRetry={fetchWorkloads}
        resetKey={sessionId}
        onOpenWorkload={(workload) => {
          useDockviewStore.getState().addBackgroundWorkPanel?.({
            sessionId: sessionId ?? undefined,
            workId: workload.work_id,
            title: workload.title,
            inCenter: true,
          });
        }}
      />
    </PanelRoot>
  );
}

function WorkloadDetailHeader({
  workload,
  onInterrupt,
  onStop,
}: {
  workload: WorkloadRunObservation;
  onInterrupt: () => void;
  onStop: () => void;
}) {
  const { t } = useTranslation();
  const canStop = canRunWorkloadAction(workload, "stop");
  const canInterrupt = canRunWorkloadAction(workload, "interrupt");
  const exitKey = workloadExitStatusKey(workload);

  return (
    <div className="shrink-0 border-b border-border" data-testid="background-work-detail-header">
      <div className="flex min-w-0 flex-wrap items-center gap-2 px-3 py-2">
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1.5">
          <Badge variant="outline" className="shrink-0 text-[10px]">
            {t(workloadKindLabelKey(workload.kind))}
          </Badge>
          <WorkloadStateBadge workload={workload} />
          {exitKey && (
            <Badge variant="outline" className="shrink-0 text-[10px] font-mono">
              {t(exitKey, { code: workload.exit_code })}
            </Badge>
          )}
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-1.5">
          {canInterrupt && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={onInterrupt}
              className="h-7 cursor-pointer px-2 text-[11px]"
              data-testid="interrupt-workload-button"
            >
              <IconPlayerPause className="mr-1 h-3 w-3" />
              {t("task:interruptBackgroundWork")}
            </Button>
          )}
          {canStop && (
            <Button
              type="button"
              variant="destructive"
              size="sm"
              onClick={onStop}
              className="h-7 cursor-pointer px-2 text-[11px]"
              data-testid="stop-workload-button"
            >
              <IconPlayerStop className="mr-1 h-3 w-3" />
              {t("task:stopBackgroundWork")}
            </Button>
          )}
          {!canStop && !canInterrupt && (
            <span
              className="text-[11px] text-muted-foreground"
              data-testid="background-work-actions-unavailable"
            >
              {t("task:backgroundActionsUnavailable")}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}

function WorkloadOutputViewer({
  workload,
  displayOutput,
  outputEndRef,
}: {
  workload: WorkloadRunObservation;
  displayOutput: string;
  outputEndRef: React.RefObject<HTMLDivElement | null>;
}) {
  const { t } = useTranslation();
  const unavailableKey = workloadOutputUnavailableKey(workload);
  return (
    <div
      className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden"
      data-testid="background-work-detail-scroll"
    >
      <div className="border-b border-border bg-background p-3">
        <p
          className="whitespace-pre-wrap break-words font-mono text-[11px] text-foreground [overflow-wrap:anywhere]"
          data-testid="background-work-detail-command"
        >
          {workload.title}
        </p>
      </div>
      <div className="bg-black/95 p-3 font-mono text-xs text-green-400">
        {workload.output_truncated && (
          <div className="mb-2 text-[10px] text-amber-400">
            [{t("task:earlierOutputTruncated")}]
          </div>
        )}
        <pre className="whitespace-pre-wrap break-all">{displayOutput || t(unavailableKey)}</pre>
        <div ref={outputEndRef} />
      </div>
    </div>
  );
}

function WorkloadOutputToolbar({
  search,
  onSearchChange,
  autoScroll,
  onToggleAutoScroll,
  onClear,
}: {
  search: string;
  onSearchChange: (v: string) => void;
  autoScroll: boolean;
  onToggleAutoScroll: () => void;
  onClear: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 flex-wrap items-center justify-between gap-2 border-b border-border px-3 py-1 text-xs text-muted-foreground">
      <div className="relative min-w-[8rem] flex-1 sm:max-w-48">
        <IconSearch
          className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder={t("task:searchOutput")}
          className="h-6 min-w-0 pl-7 text-[11px]"
        />
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onToggleAutoScroll}
          className="h-6 cursor-pointer text-[11px]"
        >
          {autoScroll ? t("task:autoScrollOn") : t("task:autoScrollOff")}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onClear}
          className="h-6 cursor-pointer text-[11px]"
        >
          <IconTrash className="mr-1 h-3 w-3" />
          {t("task:clearOutput")}
        </Button>
      </div>
    </div>
  );
}

function WorkloadInputForm({
  input,
  onInputChange,
  onSubmit,
  isSending,
}: {
  input: string;
  onInputChange: (v: string) => void;
  onSubmit: (e: React.FormEvent) => void;
  isSending: boolean;
}) {
  const { t } = useTranslation();
  return (
    <form onSubmit={onSubmit} className="flex gap-2 border-t border-border p-2">
      <Input
        value={input}
        onChange={(e) => onInputChange(e.target.value)}
        placeholder={t("task:inputPlaceholder")}
        className="h-7 min-w-0 flex-1 text-xs font-mono"
        disabled={isSending}
      />
      <Button
        type="submit"
        size="sm"
        className="h-7 shrink-0 cursor-pointer text-xs"
        disabled={isSending}
      >
        <IconSend className="mr-1 h-3.5 w-3.5" />
        {t("task:sendInput")}
      </Button>
    </form>
  );
}

function WorkloadDetailView({
  sessionId,
  workload,
  onAction,
}: {
  sessionId: string;
  workload: WorkloadRunObservation;
  onAction: (req: {
    action: "stop" | "interrupt" | "write_input";
    work_id?: string;
    data?: string;
  }) => Promise<unknown>;
}) {
  const { t } = useTranslation();
  const [input, setInput] = useState("");
  const [autoScroll, setAutoScroll] = useState(true);
  const [search, setSearch] = useState("");
  const { displayOutput, clear } = useClearedOutput(sessionId, workload, search);
  const [isSending, setIsSending] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const outputEndRef = useRef<HTMLDivElement>(null);
  const hasMountedRef = useRef(false);

  const outputAvailable = hasReadableOutput(workload.capabilities);
  useEffect(() => {
    // Keep the complete command and status visible when the detail first opens;
    // later output updates may follow the live tail.
    if (!hasMountedRef.current) {
      hasMountedRef.current = true;
      return;
    }
    if (autoScroll && outputEndRef.current) {
      outputEndRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [displayOutput, autoScroll]);

  const handleSendInput = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim() || isSending) return;
    setIsSending(true);
    setActionError(null);
    try {
      const res = await onAction({ work_id: workload.work_id, action: "write_input", data: input });
      if (
        res &&
        typeof res === "object" &&
        "success" in res &&
        !(res as { success: boolean }).success
      ) {
        setActionError((res as { error?: string }).error || t("task:actionFailed"));
      } else {
        setInput("");
      }
    } catch (err) {
      setActionError(err instanceof Error ? err.message : t("task:actionFailed"));
    } finally {
      setIsSending(false);
    }
  };

  const writeInputAvailable = canRunWorkloadAction(workload, "write_input");

  return (
    <PanelRoot data-testid={`background-work-detail-${workload.work_id}`}>
      <WorkloadDetailHeader
        workload={workload}
        onInterrupt={() => onAction({ work_id: workload.work_id, action: "interrupt" })}
        onStop={() => onAction({ work_id: workload.work_id, action: "stop" })}
      />
      <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-background">
        {outputAvailable && (
          <WorkloadOutputToolbar
            search={search}
            onSearchChange={setSearch}
            autoScroll={autoScroll}
            onToggleAutoScroll={() => setAutoScroll(!autoScroll)}
            onClear={clear}
          />
        )}

        <WorkloadOutputViewer
          workload={workload}
          displayOutput={displayOutput}
          outputEndRef={outputEndRef}
        />

        {actionError && (
          <div className="bg-destructive/10 px-3 py-1 text-[11px] text-destructive">
            {actionError}
          </div>
        )}

        {writeInputAvailable && (
          <WorkloadInputForm
            input={input}
            onInputChange={setInput}
            onSubmit={handleSendInput}
            isSending={isSending}
          />
        )}
      </div>
    </PanelRoot>
  );
}
