"use client";

import { useState } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import type { RunSessionDetail } from "@/lib/api/domains/office-extended-api";

type Props = {
  session: RunSessionDetail;
};

/**
 * Session collapsible — surfaces the session ids associated with
 * the run. session_id_before / session_id_after are spec-level
 * placeholders we don't track explicitly yet, so v1 only renders
 * the run's claimed session id. The "Reset session for touched
 * tasks" action is out of scope for v1.
 */
export function SessionCollapsible({ session }: Props) {
  const [open, setOpen] = useState(false);
  const hasAny = Boolean(
    session.session_id || session.session_id_before || session.session_id_after,
  );
  if (!hasAny) return null;
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="rounded-lg border border-border">
      <CollapsibleTrigger
        className="w-full flex items-center justify-between px-4 py-2 cursor-pointer hover:bg-muted/40"
        data-testid="run-session-collapsible-trigger"
      >
        <span className="text-sm font-medium">Session</span>
        <IconChevronDown className={`h-4 w-4 transition-transform ${open ? "rotate-180" : ""}`} />
      </CollapsibleTrigger>
      <CollapsibleContent className="px-4 py-3 space-y-1.5 border-t border-border text-xs">
        {session.session_id && (
          <Row label="Session ID" value={session.session_id} testid="session-id" />
        )}
        {session.session_id_before && (
          <Row
            label="Session ID (before)"
            value={session.session_id_before}
            testid="session-id-before"
          />
        )}
        {session.session_id_after && (
          <Row
            label="Session ID (after)"
            value={session.session_id_after}
            testid="session-id-after"
          />
        )}
      </CollapsibleContent>
    </Collapsible>
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
