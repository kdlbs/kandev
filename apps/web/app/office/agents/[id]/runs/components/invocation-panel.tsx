"use client";

import { useState } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import type { RunInvocationDetail } from "@/lib/api/domains/office-extended-api";

type Props = {
  invocation: RunInvocationDetail;
};

/**
 * Invocation panel — surfaces the adapter family + working directory
 * up front; command / env land inside a Details collapsible when
 * available. Empty fields are hidden so the panel stays compact for
 * runs where the orchestrator didn't capture them.
 */
export function InvocationPanel({ invocation }: Props) {
  const [open, setOpen] = useState(false);
  const hasDetails = Boolean(
    invocation.command || (invocation.env && Object.keys(invocation.env).length > 0),
  );
  if (!invocation.adapter && !invocation.model && !invocation.working_dir && !hasDetails) {
    return null;
  }
  return (
    <div className="rounded-lg border border-border" data-testid="invocation-panel">
      <div className="px-4 py-3 space-y-1.5 text-xs">
        {invocation.adapter && (
          <Row label="Adapter" value={invocation.adapter} testid="invocation-adapter" />
        )}
        {invocation.model && (
          <Row label="Model" value={invocation.model} testid="invocation-model" />
        )}
        {invocation.working_dir && (
          <Row label="Working dir" value={invocation.working_dir} testid="invocation-cwd" />
        )}
      </div>
      {hasDetails && (
        <Collapsible open={open} onOpenChange={setOpen} className="border-t border-border">
          <CollapsibleTrigger
            className="w-full flex items-center justify-between px-4 py-2 cursor-pointer hover:bg-muted/40"
            data-testid="invocation-details-trigger"
          >
            <span className="text-sm font-medium">Details</span>
            <IconChevronDown
              className={`h-4 w-4 transition-transform ${open ? "rotate-180" : ""}`}
            />
          </CollapsibleTrigger>
          <CollapsibleContent className="px-4 py-3 text-xs space-y-2">
            {invocation.command && (
              <pre
                className="font-mono whitespace-pre-wrap break-words bg-muted/40 rounded p-2"
                data-testid="invocation-command"
              >
                {invocation.command}
              </pre>
            )}
            {invocation.env && Object.keys(invocation.env).length > 0 && (
              <div className="space-y-0.5" data-testid="invocation-env">
                {Object.entries(invocation.env).map(([k, v]) => (
                  <div key={k} className="font-mono">
                    <span className="text-muted-foreground">{k}=</span>
                    <span>{v}</span>
                  </div>
                ))}
              </div>
            )}
          </CollapsibleContent>
        </Collapsible>
      )}
    </div>
  );
}

function Row({ label, value, testid }: { label: string; value: string; testid: string }) {
  return (
    <div className="flex items-center gap-2" data-testid={testid}>
      <span className="text-muted-foreground w-32 flex-shrink-0">{label}</span>
      <span className="font-mono break-all">{value}</span>
    </div>
  );
}
