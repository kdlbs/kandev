"use client";

import Link from "next/link";
import { Badge } from "@kandev/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import type { AgentRoutePreview } from "@/lib/state/slices/office/types";
import { providerLabel } from "./provider-order-editor";

type Props = {
  agents: AgentRoutePreview[];
  isLoading?: boolean;
};

export function AgentPreviewTable({ agents, isLoading }: Props) {
  const sorted = [...agents].sort((a, b) => a.agent_name.localeCompare(b.agent_name));
  return (
    <div className="rounded-lg border border-border overflow-hidden">
      <div className="px-4 py-3 border-b border-border">
        <p className="text-sm font-medium">Resolved routes</p>
        <p className="text-xs text-muted-foreground mt-0.5">
          What each agent would launch with the current settings.
        </p>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Agent</TableHead>
            <TableHead>Tier</TableHead>
            <TableHead>Primary</TableHead>
            <TableHead>Fallback</TableHead>
            <TableHead>Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sorted.length === 0 ? (
            <TableRow>
              <TableCell colSpan={5} className="text-center text-xs text-muted-foreground py-6">
                {isLoading ? "Loading…" : "No agents in this workspace yet."}
              </TableCell>
            </TableRow>
          ) : (
            sorted.map((a) => <PreviewRow key={a.agent_id} a={a} />)
          )}
        </TableBody>
      </Table>
    </div>
  );
}

function PreviewRow({ a }: { a: AgentRoutePreview }) {
  return (
    <TableRow data-testid={`preview-row-${a.agent_id}`}>
      <TableCell>
        <Link
          href={`/office/agents/${a.agent_id}/configuration`}
          className="cursor-pointer underline-offset-4 hover:underline"
        >
          {a.agent_name}
        </Link>
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-1.5">
          <Badge variant="secondary" className="capitalize">
            {a.effective_tier}
          </Badge>
          <span className="text-[10px] text-muted-foreground uppercase tracking-wide">
            {a.tier_source}
          </span>
        </div>
      </TableCell>
      <TableCell>
        {a.primary_provider_id ? (
          <span className="text-xs font-mono">
            {providerLabel(a.primary_provider_id)} / {a.primary_model || "—"}
          </span>
        ) : (
          <span className="text-xs text-muted-foreground italic">none</span>
        )}
      </TableCell>
      <TableCell>
        <FallbackChain chain={a.fallback_chain} />
      </TableCell>
      <TableCell>
        <StatusBadges missing={a.missing} degraded={a.degraded} />
      </TableCell>
    </TableRow>
  );
}

function FallbackChain({ chain }: { chain: AgentRoutePreview["fallback_chain"] }) {
  if (!chain || chain.length === 0) {
    return <span className="text-xs text-muted-foreground">—</span>;
  }
  return (
    <div className="flex flex-wrap gap-1 text-[11px] font-mono">
      {chain.map((p, i) => (
        <span key={`${p.provider_id}-${i}`} className="text-muted-foreground">
          {i > 0 && <span className="px-1">→</span>}
          {providerLabel(p.provider_id)}/{p.model || "?"}
        </span>
      ))}
    </div>
  );
}

function StatusBadges({ missing, degraded }: { missing: string[]; degraded: boolean }) {
  if (missing.length === 0 && !degraded) {
    return <Badge variant="outline">Ready</Badge>;
  }
  return (
    <div className="flex flex-wrap gap-1">
      {degraded && <Badge variant="destructive">Degraded</Badge>}
      {missing.length > 0 && (
        <Badge variant="outline" title={missing.join(", ")}>
          {missing.length} missing
        </Badge>
      )}
    </div>
  );
}
