"use client";

import Link from "next/link";
import { Card } from "@kandev/ui/card";
import { Badge } from "@kandev/ui/badge";
import { useAppStore } from "@/components/state-provider";
import { useProviderHealth } from "@/hooks/domains/office/use-provider-health";
import { useWorkspaceRouting } from "@/hooks/domains/office/use-workspace-routing";
import type { ProviderHealth, ProviderHealthState } from "@/lib/state/slices/office/types";
import { providerLabel } from "../../workspace/routing/components/provider-order-editor";

const STATE_PILL: Record<
  ProviderHealthState,
  { label: string; variant: "default" | "secondary" | "destructive" | "outline" }
> = {
  healthy: { label: "Healthy", variant: "secondary" },
  degraded: { label: "Degraded", variant: "destructive" },
  user_action_required: { label: "Needs action", variant: "destructive" },
};

export function ProviderHealthCard() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const workspace = useWorkspaceRouting(workspaceId);
  const { health } = useProviderHealth(workspaceId);
  if (!workspace.config?.enabled) return null;

  const byProvider = collapseByProvider(health);
  const order = workspace.config.provider_order;
  const rows =
    order.length > 0
      ? order.map((p) => byProvider.get(p) ?? mkEmptyHealthy(p))
      : [...byProvider.values()];

  return (
    <Card>
      <div className="p-4 border-b border-border flex items-center justify-between">
        <h2 className="text-sm font-semibold">Provider Health</h2>
        <Link
          href="/office/workspace/routing"
          className="text-xs underline-offset-4 hover:underline cursor-pointer text-muted-foreground"
        >
          Manage
        </Link>
      </div>
      <div className="divide-y divide-border">
        {rows.length === 0 ? (
          <div className="px-4 py-6 text-center text-sm text-muted-foreground">
            No providers configured.
          </div>
        ) : (
          rows.map((h) => <ProviderHealthRow key={h.provider_id} h={h} />)
        )}
      </div>
    </Card>
  );
}

function ProviderHealthRow({ h }: { h: ProviderHealth }) {
  const pill = STATE_PILL[h.state];
  return (
    <Link
      href="/office/workspace/routing"
      className="flex items-center gap-3 px-4 py-2.5 hover:bg-accent/50 transition-colors cursor-pointer"
    >
      <span className="text-sm flex-1">{providerLabel(h.provider_id)}</span>
      <Badge variant={pill.variant} className="text-xs">
        {pill.label}
      </Badge>
      {h.error_code && (
        <span className="text-[10px] font-mono text-muted-foreground">{h.error_code}</span>
      )}
    </Link>
  );
}

function collapseByProvider(rows: ProviderHealth[]): Map<string, ProviderHealth> {
  const out = new Map<string, ProviderHealth>();
  for (const r of rows) {
    const existing = out.get(r.provider_id);
    if (!existing || severity(r.state) > severity(existing.state)) {
      out.set(r.provider_id, r);
    }
  }
  return out;
}

function severity(s: ProviderHealthState): number {
  if (s === "user_action_required") return 2;
  if (s === "degraded") return 1;
  return 0;
}

function mkEmptyHealthy(providerId: string): ProviderHealth {
  return {
    provider_id: providerId,
    scope: "provider",
    scope_value: "",
    state: "healthy",
    backoff_step: 0,
  };
}
