"use client";

import { useState, useRef, useEffect, useMemo } from "react";
import {
  IconPlayerStop,
  IconPlayerPause,
  IconSearch,
  IconSend,
  IconTrash,
  IconTerminal2,
  IconCpu,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { useTranslation } from "react-i18next";
import { PanelRoot, PanelBody, PanelHeaderBarSplit } from "@/components/task/panel-primitives";
import { useAppStore } from "@/components/state-provider";
import { useBackgroundWork } from "@/hooks/domains/session/use-background-work";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import { cn } from "@/lib/utils";

export type BackgroundWorkPanelProps = {
  panelId: string;
  params?: Record<string, unknown>;
};

export function BackgroundWorkPanel({ params }: BackgroundWorkPanelProps) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionId =
    typeof params?.sessionId === "string" && params.sessionId ? params.sessionId : activeSessionId;
  const workId = typeof params?.workId === "string" ? params.workId : undefined;
  const { workloads, isLoading, executeAction } = useBackgroundWork(sessionId);

  const selectedWorkload = useMemo(() => {
    if (!workId) return null;
    return workloads.find((w) => w.work_id === workId) ?? null;
  }, [workloads, workId]);

  if (selectedWorkload) {
    return <WorkloadDetailView workload={selectedWorkload} onAction={executeAction} />;
  }

  return <WorkloadOverviewView workloads={workloads} isLoading={isLoading} />;
}

function WorkloadOverviewBody({
  isLoading,
  workloads,
  filtered,
}: {
  isLoading: boolean;
  workloads: WorkloadRunObservation[];
  filtered: WorkloadRunObservation[];
}) {
  const { t } = useTranslation();
  if (isLoading && workloads.length === 0) {
    return (
      <div className="py-8 text-center text-xs text-muted-foreground">
        {t("task:loadingBackgroundWork")}
      </div>
    );
  }
  if (filtered.length === 0) {
    return (
      <div className="py-8 text-center text-xs text-muted-foreground">
        {t("task:noBackgroundWork")}
      </div>
    );
  }
  return (
    <div className="grid gap-2">
      {filtered.map((w) => (
        <WorkloadCard key={w.work_id} workload={w} />
      ))}
    </div>
  );
}

function WorkloadOverviewView({
  workloads,
  isLoading,
}: {
  workloads: WorkloadRunObservation[];
  isLoading: boolean;
}) {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    if (!search.trim()) return workloads;
    const q = search.toLowerCase();
    return workloads.filter(
      (w) => w.title.toLowerCase().includes(q) || w.kind.toLowerCase().includes(q),
    );
  }, [workloads, search]);

  return (
    <PanelRoot data-testid="background-work-overview-panel">
      <PanelHeaderBarSplit
        left={
          <div className="flex items-center gap-2">
            <IconTerminal2 className="h-4 w-4 text-primary" />
            <span className="font-semibold">{t("task:backgroundWorkOverview")}</span>
            <Badge variant="outline" className="text-[10px]">
              {workloads.length}
            </Badge>
          </div>
        }
        right={
          <div className="relative w-48">
            <IconSearch className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t("task:searchOutput")}
              className="h-7 pl-7 text-xs"
            />
          </div>
        }
      />
      <PanelBody padding scroll className="space-y-2">
        <WorkloadOverviewBody isLoading={isLoading} workloads={workloads} filtered={filtered} />
      </PanelBody>
    </PanelRoot>
  );
}

function WorkloadCard({ workload }: { workload: WorkloadRunObservation }) {
  const { t } = useTranslation();
  const handleOpen = () => {
    useDockviewStore.getState().addBackgroundWorkPanel?.({
      workId: workload.work_id,
      title: workload.title,
      inCenter: true,
    });
  };

  return (
    <div
      data-testid={`background-workload-card-${workload.work_id}`}
      className="flex items-center justify-between gap-3 rounded-lg border border-border bg-card/60 p-3 shadow-sm transition-colors hover:bg-muted/40"
    >
      <div className="flex min-w-0 flex-1 items-center gap-2.5">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          {workload.kind === "subagent" ? (
            <IconCpu className="h-4 w-4" />
          ) : (
            <IconTerminal2 className="h-4 w-4" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate font-medium text-sm text-foreground">{workload.title}</span>
            <Badge
              variant={workload.state === "running" ? "default" : "outline"}
              className="text-[10px] capitalize"
            >
              {workload.state}
            </Badge>
          </div>
          <span className="text-[11px] text-muted-foreground capitalize">{workload.kind}</span>
        </div>
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="h-7 cursor-pointer text-xs"
        onClick={handleOpen}
        data-testid={`open-workload-${workload.work_id}`}
      >
        {t("task:openBackgroundWork")}
      </Button>
    </div>
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
  const stopCap = workload.capabilities?.actions?.stop;
  const canStop =
    stopCap !== undefined
      ? stopCap.supported && stopCap.available
      : workload.state === "running" || workload.state === "waiting";
  const intCap = workload.capabilities?.actions?.interrupt;
  const canInterrupt =
    intCap !== undefined
      ? intCap.supported && intCap.available
      : workload.kind === "subagent" &&
        (workload.state === "running" || workload.state === "waiting");

  return (
    <PanelHeaderBarSplit
      left={
        <div className="flex items-center gap-2">
          <span className="font-semibold">{workload.title}</span>
          <Badge variant="outline" className="text-[10px] capitalize">
            {workload.kind}
          </Badge>
          <Badge
            variant={workload.state === "running" ? "default" : "outline"}
            className={cn(
              "text-[10px] capitalize",
              workload.state === "running" && "bg-primary text-primary-foreground",
            )}
          >
            {workload.state}
          </Badge>
        </div>
      }
      right={
        <div className="flex items-center gap-1.5">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!canInterrupt}
            onClick={onInterrupt}
            className="h-6 cursor-pointer px-2 text-[11px]"
            data-testid="interrupt-workload-button"
          >
            <IconPlayerPause className="mr-1 h-3 w-3" />
            {t("task:interruptBackgroundWork")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            size="sm"
            disabled={!canStop}
            onClick={onStop}
            className="h-6 cursor-pointer px-2 text-[11px]"
            data-testid="stop-workload-button"
          >
            <IconPlayerStop className="mr-1 h-3 w-3" />
            {t("task:stopBackgroundWork")}
          </Button>
        </div>
      }
    />
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
  return (
    <div className="flex-1 min-h-0 overflow-y-auto bg-black/95 p-3 font-mono text-xs text-green-400">
      {workload.output_truncated && (
        <div className="mb-2 text-[10px] text-amber-400">[{t("task:earlierOutputTruncated")}]</div>
      )}
      <pre className="whitespace-pre-wrap break-all">
        {displayOutput || t("task:liveOutputUnavailable")}
      </pre>
      <div ref={outputEndRef} />
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
    <div className="flex items-center justify-between border-b border-border px-3 py-1 text-xs text-muted-foreground">
      <div className="relative w-48">
        <IconSearch className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder={t("task:searchOutput")}
          className="h-6 pl-7 text-[11px]"
        />
      </div>
      <div className="flex items-center gap-2">
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
        className="h-7 text-xs font-mono"
        disabled={isSending}
      />
      <Button type="submit" size="sm" className="h-7 cursor-pointer text-xs" disabled={isSending}>
        <IconSend className="mr-1 h-3.5 w-3.5" />
        {t("task:sendInput")}
      </Button>
    </form>
  );
}

function WorkloadDetailView({
  workload,
  onAction,
}: {
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
  const [clearedOffset, setClearedOffset] = useState<number>(0);
  const [isSending, setIsSending] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const outputEndRef = useRef<HTMLDivElement>(null);

  const displayOutput = useMemo(() => {
    let text = workload.output || "";
    if (clearedOffset > 0) {
      text = text.slice(clearedOffset);
    }
    if (!search.trim()) return text;
    const q = search.toLowerCase();
    return text
      .split("\n")
      .filter((line) => line.toLowerCase().includes(q))
      .join("\n");
  }, [workload.output, clearedOffset, search]);

  useEffect(() => {
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

  const writeInputAvailable = workload.capabilities?.actions?.write_input?.available ?? false;

  return (
    <PanelRoot data-testid={`background-work-detail-${workload.work_id}`}>
      <WorkloadDetailHeader
        workload={workload}
        onInterrupt={() => onAction({ work_id: workload.work_id, action: "interrupt" })}
        onStop={() => onAction({ work_id: workload.work_id, action: "stop" })}
      />
      <div className="flex flex-1 min-h-0 flex-col bg-background">
        <WorkloadOutputToolbar
          search={search}
          onSearchChange={setSearch}
          autoScroll={autoScroll}
          onToggleAutoScroll={() => setAutoScroll(!autoScroll)}
          onClear={() => setClearedOffset((workload.output || "").length)}
        />

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
