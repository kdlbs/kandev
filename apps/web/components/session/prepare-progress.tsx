"use client";

import { useState } from "react";
import {
  IconCheck,
  IconX,
  IconLoader2,
  IconAlertTriangle,
  IconChevronDown,
  IconChevronRight,
} from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import type { PrepareStepInfo } from "@/lib/state/slices/session-runtime/types";

type PrepareProgressProps = {
  sessionId: string;
};

function StepIcon({ status, hasWarning }: { status: string; hasWarning?: boolean }) {
  if (status === "completed" && hasWarning) {
    return <IconAlertTriangle className="h-3.5 w-3.5 text-amber-500" />;
  }
  if (status === "completed") {
    return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
  }
  if (status === "failed") {
    return <IconX className="h-3.5 w-3.5 text-destructive" />;
  }
  if (status === "running") {
    return <IconLoader2 className="h-3.5 w-3.5 text-muted-foreground animate-spin" />;
  }
  return <div className="h-3.5 w-3.5 rounded-full border border-muted-foreground/30" />;
}

function StepRow({ step }: { step: PrepareStepInfo }) {
  const [detailExpanded, setDetailExpanded] = useState(false);
  return (
    <div className="flex items-start gap-2 text-xs">
      <div className="mt-0.5 flex-shrink-0">
        <StepIcon status={step.status} hasWarning={Boolean(step.warning)} />
      </div>
      <div className="min-w-0 flex-1">
        <span className="text-muted-foreground">{step.name || "Preparing..."}</span>
        {step.output && (
          <pre className="text-muted-foreground/60 mt-0.5 overflow-x-auto whitespace-pre text-xs">
            {step.output}
          </pre>
        )}
        {step.warning && (
          <div className="mt-0.5">
            <pre className="text-amber-500 overflow-x-auto whitespace-pre text-xs">
              {step.warning}
            </pre>
            {step.warningDetail && (
              <>
                <button
                  type="button"
                  className="flex items-center gap-0.5 text-amber-500/70 hover:text-amber-500 mt-0.5 cursor-pointer"
                  onClick={() => setDetailExpanded((v) => !v)}
                >
                  {detailExpanded ? (
                    <IconChevronDown className="h-3 w-3" />
                  ) : (
                    <IconChevronRight className="h-3 w-3" />
                  )}
                  <span>Details</span>
                </button>
                {detailExpanded && (
                  <pre className="text-amber-500/70 mt-0.5 overflow-x-auto whitespace-pre text-xs">
                    {step.warningDetail}
                  </pre>
                )}
              </>
            )}
          </div>
        )}
        {step.error && (
          <pre className="text-destructive mt-0.5 overflow-x-auto whitespace-pre text-xs">
            {step.error}
          </pre>
        )}
      </div>
    </div>
  );
}

function useEffectivePrepareStatus(sessionId: string) {
  const prepareState = useAppStore((state) => state.prepareProgress.bySessionId[sessionId] ?? null);
  const sessionState = useAppStore((state) => state.taskSessions.items[sessionId]?.state);
  const agentctlStatus = useAppStore(
    (state) => state.sessionAgentctl.itemsBySessionId[sessionId]?.status,
  );
  const profileLabel = useAppStore((state) => {
    const session = state.taskSessions.items[sessionId];
    if (!session?.agent_profile_id) return null;
    const profile = state.agentProfiles.items.find((p) => p.id === session.agent_profile_id);
    return profile?.label ?? null;
  });

  const hasWarnings = prepareState?.steps.some((s) => s.warning);

  if (!prepareState)
    return {
      visible: false,
      status: "preparing",
      prepareState,
      profileLabel,
      hasWarnings: false,
    } as const;
  if (prepareState.status === "completed" && !hasWarnings)
    return {
      visible: false,
      status: "completed",
      prepareState,
      profileLabel,
      hasWarnings: false,
    } as const;
  if (prepareState.status === "completed" && hasWarnings)
    return {
      visible: true,
      status: "completed_with_warnings",
      prepareState,
      profileLabel,
      hasWarnings: true,
    } as const;

  // Agentctl ready implies environment preparation succeeded —
  // dismiss even if the completed event hasn't arrived yet.
  if (agentctlStatus === "ready")
    return { visible: false, status: "completed", prepareState, profileLabel } as const;

  // If session reached a terminal state but prepare status is still "preparing",
  // treat it as failed (the completed event may not have arrived)
  const isSessionTerminal =
    sessionState === "FAILED" || sessionState === "COMPLETED" || sessionState === "CANCELLED";
  const status =
    prepareState.status === "preparing" && isSessionTerminal ? "failed" : prepareState.status;

  return {
    visible: true,
    status,
    prepareState,
    profileLabel,
    hasWarnings: hasWarnings ?? false,
  } as const;
}

export function PrepareProgress({ sessionId }: PrepareProgressProps) {
  const { visible, status, prepareState, profileLabel } = useEffectivePrepareStatus(sessionId);
  const [dismissed, setDismissed] = useState(false);
  const [expandedDetails, setExpandedDetails] = useState<Record<number, boolean>>({});

  if (!visible || !prepareState) return null;

  // After successful completion with warnings, show only the warning steps.
  if (status === "completed_with_warnings") {
    if (dismissed) return null;
    const warningSteps = prepareState.steps
      .map((s, i) => ({ step: s, index: i }))
      .filter(({ step }) => step.warning);
    return (
      <div data-testid="prepare-warning-banner" className="px-3 py-1.5 border-b border-border/50">
        {warningSteps.map(({ step, index }) => (
          <div key={index} className="text-xs text-amber-500">
            <div className="flex items-center gap-1.5">
              <IconAlertTriangle className="h-3 w-3 flex-shrink-0" />
              <span className="flex-1">{step.warning}</span>
              {step.warningDetail && (
                <button
                  type="button"
                  className="flex items-center gap-0.5 text-amber-500/70 hover:text-amber-500 cursor-pointer"
                  onClick={() => setExpandedDetails((prev) => ({ ...prev, [index]: !prev[index] }))}
                >
                  {expandedDetails[index] ? (
                    <IconChevronDown className="h-3 w-3" />
                  ) : (
                    <IconChevronRight className="h-3 w-3" />
                  )}
                  <span>Details</span>
                </button>
              )}
              <button
                type="button"
                className="text-amber-500/70 hover:text-amber-500 ml-1 cursor-pointer"
                onClick={() => setDismissed(true)}
                aria-label="Dismiss warning"
              >
                <IconX className="h-3 w-3" />
              </button>
            </div>
            {step.warningDetail && expandedDetails[index] && (
              <pre className="mt-1 ml-4.5 text-amber-500/70 overflow-x-auto whitespace-pre text-xs">
                {step.warningDetail}
              </pre>
            )}
          </div>
        ))}
      </div>
    );
  }

  const visibleSteps = prepareState.steps.filter(
    (step) =>
      step.name.trim() !== "" ||
      Boolean(step.output) ||
      Boolean(step.error) ||
      Boolean(step.warning),
  );

  return (
    <div className="px-3 py-2 space-y-1.5 border-b border-border/50">
      <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        {status === "preparing" && (
          <IconLoader2
            data-testid="prepare-progress-header-spinner"
            className="h-3.5 w-3.5 animate-spin"
          />
        )}
        {status === "failed" && <IconX className="h-3.5 w-3.5 text-destructive" />}
        <span>
          {status === "preparing" && "Preparing environment..."}
          {status === "failed" && (prepareState.errorMessage ?? "Environment preparation failed")}
        </span>
        {profileLabel && (
          <span className="text-muted-foreground/50 ml-auto font-normal">{profileLabel}</span>
        )}
      </div>
      {visibleSteps.length > 0 && (
        <div className="space-y-1 pl-1">
          {visibleSteps.map((step, i) => (
            <StepRow key={i} step={step} />
          ))}
        </div>
      )}
    </div>
  );
}
