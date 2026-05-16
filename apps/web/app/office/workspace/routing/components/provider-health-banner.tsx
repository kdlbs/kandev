"use client";

import { useState } from "react";
import { IconAlertTriangle, IconCircleCheck, IconRefresh } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import type { ProviderHealth } from "@/lib/state/slices/office/types";
import { providerLabel } from "./provider-order-editor";

type Props = {
  health: ProviderHealth[];
  onRetry: (providerId: string) => Promise<void>;
};

const STATE_BADGE: Record<ProviderHealth["state"], { label: string; variant: BadgeVariant }> = {
  healthy: { label: "Healthy", variant: "outline" },
  degraded: { label: "Degraded", variant: "destructive" },
  user_action_required: { label: "Needs action", variant: "destructive" },
};

type BadgeVariant = "default" | "secondary" | "destructive" | "outline";

export function ProviderHealthBanner({ health, onRetry }: Props) {
  const nonHealthy = health.filter((h) => h.state !== "healthy");
  if (nonHealthy.length === 0) {
    return (
      <div className="rounded-lg border border-border p-3 flex items-center gap-2 text-sm">
        <IconCircleCheck className="h-4 w-4 text-emerald-600" />
        <span>All providers healthy.</span>
      </div>
    );
  }
  return (
    <div className="rounded-lg border border-amber-300 bg-amber-50 dark:bg-amber-950/30 divide-y divide-amber-300 dark:divide-amber-900">
      {nonHealthy.map((h) => (
        <ProviderHealthRow
          key={`${h.provider_id}:${h.scope}:${h.scope_value}`}
          h={h}
          onRetry={onRetry}
        />
      ))}
    </div>
  );
}

function ProviderHealthRow({
  h,
  onRetry,
}: {
  h: ProviderHealth;
  onRetry: (providerId: string) => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const meta = STATE_BADGE[h.state];
  const handleRetry = async () => {
    if (busy) return;
    setBusy(true);
    try {
      await onRetry(h.provider_id);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="flex items-center gap-3 px-3 py-2 text-sm">
      <IconAlertTriangle className="h-4 w-4 text-amber-700 dark:text-amber-300 shrink-0" />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="font-medium">{providerLabel(h.provider_id)}</span>
          <Badge variant={meta.variant}>{meta.label}</Badge>
          {h.error_code && (
            <span className="text-xs font-mono text-muted-foreground">{h.error_code}</span>
          )}
        </div>
        {h.retry_at && <p className="text-xs text-muted-foreground">Retry at {h.retry_at}</p>}
      </div>
      <Button
        size="sm"
        variant="outline"
        onClick={handleRetry}
        disabled={busy}
        className="cursor-pointer gap-1"
      >
        <IconRefresh className="h-3.5 w-3.5" />
        {busy ? "Retrying…" : "Retry"}
      </Button>
    </div>
  );
}
