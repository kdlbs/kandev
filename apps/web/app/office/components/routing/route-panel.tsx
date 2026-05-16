"use client";

import { useState } from "react";
import {
  IconCheck,
  IconChevronDown,
  IconChevronRight,
  IconX,
  IconAlertCircle,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { useRunAttempts } from "@/hooks/domains/office/use-run-attempts";
import type { RouteAttempt, RouteAttemptOutcome } from "@/lib/state/slices/office/types";
import { providerLabel } from "../../workspace/routing/components/provider-order-editor";

type Props = { runId: string };

const OUTCOME_VARIANT: Record<
  RouteAttemptOutcome,
  "default" | "secondary" | "destructive" | "outline"
> = {
  launched: "default",
  failed_provider_unavailable: "destructive",
  failed_other: "destructive",
  skipped_degraded: "outline",
  skipped_user_action: "outline",
  skipped_missing_mapping: "outline",
  skipped_max_attempts: "destructive",
};

export function RoutePanel({ runId }: Props) {
  const { attempts, isLoading } = useRunAttempts(runId);
  if (!isLoading && attempts.length === 0) return null;
  return (
    <div className="rounded-lg border border-border" data-testid="route-panel">
      <div className="px-4 py-3 border-b border-border">
        <p className="text-sm font-medium">Route attempts</p>
        <p className="text-xs text-muted-foreground mt-0.5">
          Provider candidates tried for this run, in order.
        </p>
      </div>
      <ul className="divide-y divide-border">
        {attempts.map((a) => (
          <li key={a.seq}>
            <RouteAttemptRow attempt={a} />
          </li>
        ))}
      </ul>
    </div>
  );
}

function RouteAttemptRow({ attempt }: { attempt: RouteAttempt }) {
  const [open, setOpen] = useState(false);
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <div className="flex items-center gap-2 px-4 py-2 text-sm">
        <span className="text-xs text-muted-foreground tabular-nums w-6">#{attempt.seq}</span>
        <OutcomeIcon outcome={attempt.outcome} />
        <span className="font-mono">
          {providerLabel(attempt.provider_id)}/{attempt.model || "?"}
        </span>
        <Badge variant="secondary" className="capitalize text-[10px]">
          {attempt.tier || "—"}
        </Badge>
        <Badge variant={OUTCOME_VARIANT[attempt.outcome] ?? "outline"} className="text-[10px]">
          {humanOutcome(attempt.outcome)}
        </Badge>
        {attempt.error_code && (
          <span className="text-xs font-mono text-muted-foreground">{attempt.error_code}</span>
        )}
        <span className="text-xs text-muted-foreground ml-auto">
          {durationLabel(attempt.started_at, attempt.finished_at)}
        </span>
        <CollapsibleTrigger asChild>
          <Button variant="ghost" size="icon" className="h-6 w-6 cursor-pointer">
            {open ? (
              <IconChevronDown className="h-3.5 w-3.5" />
            ) : (
              <IconChevronRight className="h-3.5 w-3.5" />
            )}
          </Button>
        </CollapsibleTrigger>
      </div>
      <CollapsibleContent>
        <DebugBlock attempt={attempt} />
      </CollapsibleContent>
    </Collapsible>
  );
}

function OutcomeIcon({ outcome }: { outcome: RouteAttemptOutcome }) {
  if (outcome === "launched") {
    return <IconCheck className="h-3.5 w-3.5 text-emerald-600" />;
  }
  if (outcome.startsWith("failed")) {
    return <IconX className="h-3.5 w-3.5 text-red-600" />;
  }
  return <IconAlertCircle className="h-3.5 w-3.5 text-amber-600" />;
}

function humanOutcome(outcome: RouteAttemptOutcome): string {
  return outcome.replace(/_/g, " ");
}

function durationLabel(start: string, end?: string): string {
  if (!end) return "in flight";
  const ms = Date.parse(end) - Date.parse(start);
  if (Number.isNaN(ms) || ms < 0) return "—";
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function DebugBlock({ attempt }: { attempt: RouteAttempt }) {
  return (
    <div className="px-10 py-2 text-xs font-mono space-y-1 bg-muted/30 border-t border-border">
      {attempt.error_confidence && <KV k="confidence" v={attempt.error_confidence} />}
      {attempt.adapter_phase && <KV k="phase" v={attempt.adapter_phase} />}
      {attempt.classifier_rule && <KV k="rule" v={attempt.classifier_rule} />}
      {typeof attempt.exit_code === "number" && <KV k="exit_code" v={String(attempt.exit_code)} />}
      {attempt.reset_hint && <KV k="reset_hint" v={attempt.reset_hint} />}
      {attempt.raw_excerpt && (
        <pre className="whitespace-pre-wrap break-words text-[10px] text-muted-foreground">
          {attempt.raw_excerpt}
        </pre>
      )}
    </div>
  );
}

function KV({ k, v }: { k: string; v: string }) {
  return (
    <div>
      <span className="text-muted-foreground">{k}:</span> {v}
    </div>
  );
}
