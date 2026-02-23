"use client";

import { IconCheck, IconX, IconLoader2 } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import type { PrepareStepInfo } from "@/lib/state/slices/session-runtime/types";

type PrepareProgressProps = {
  sessionId: string;
};

function StepIcon({ status }: { status: string }) {
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
  return (
    <div className="flex items-start gap-2 text-xs">
      <div className="mt-0.5 flex-shrink-0">
        <StepIcon status={step.status} />
      </div>
      <div className="min-w-0 flex-1">
        <span className="text-muted-foreground">{step.name || "Preparing..."}</span>
        {step.output && (
          <pre className="text-muted-foreground/60 mt-0.5 overflow-x-auto whitespace-pre text-xs">
            {step.output}
          </pre>
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
  const profileLabel = useAppStore((state) => {
    const session = state.taskSessions.items[sessionId];
    if (!session?.agent_profile_id) return null;
    const profile = state.agentProfiles.items.find((p) => p.id === session.agent_profile_id);
    return profile?.label ?? null;
  });

  if (!prepareState)
    return { visible: false, status: "preparing", prepareState, profileLabel } as const;
  if (prepareState.status === "completed")
    return { visible: false, status: "completed", prepareState, profileLabel } as const;

  // If session reached a terminal state but prepare status is still "preparing",
  // treat it as failed (the completed event may not have arrived)
  const isSessionTerminal =
    sessionState === "FAILED" || sessionState === "COMPLETED" || sessionState === "CANCELLED";
  const status =
    prepareState.status === "preparing" && isSessionTerminal ? "failed" : prepareState.status;

  return { visible: true, status, prepareState, profileLabel } as const;
}

export function PrepareProgress({ sessionId }: PrepareProgressProps) {
  const { visible, status, prepareState, profileLabel } = useEffectivePrepareStatus(sessionId);

  if (!visible || !prepareState) return null;
  const visibleSteps = prepareState.steps.filter(
    (step) => step.name.trim() !== "" || Boolean(step.output) || Boolean(step.error),
  );

  return (
    <div className="px-3 py-2 space-y-1.5 border-b border-border/50">
      <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        {status === "preparing" && <IconLoader2 className="h-3 w-3 animate-spin" />}
        {status === "failed" && <IconX className="h-3 w-3 text-destructive" />}
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
