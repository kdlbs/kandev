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
          <div className="text-muted-foreground/60 mt-0.5 truncate">{step.output}</div>
        )}
        {step.error && <div className="text-destructive mt-0.5 truncate">{step.error}</div>}
      </div>
    </div>
  );
}

export function PrepareProgress({ sessionId }: PrepareProgressProps) {
  const prepareState = useAppStore((state) => state.prepareProgress.bySessionId[sessionId] ?? null);

  if (!prepareState || prepareState.status === "completed") return null;

  return (
    <div className="px-3 py-2 space-y-1.5 border-b border-border/50">
      <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        {prepareState.status === "preparing" && <IconLoader2 className="h-3 w-3 animate-spin" />}
        {prepareState.status === "failed" && <IconX className="h-3 w-3 text-destructive" />}
        <span>
          {prepareState.status === "preparing" && "Preparing environment..."}
          {prepareState.status === "failed" &&
            (prepareState.errorMessage ?? "Environment preparation failed")}
        </span>
      </div>
      {prepareState.steps.length > 0 && (
        <div className="space-y-1 pl-1">
          {prepareState.steps.map((step, i) => (
            <StepRow key={i} step={step} />
          ))}
        </div>
      )}
    </div>
  );
}
