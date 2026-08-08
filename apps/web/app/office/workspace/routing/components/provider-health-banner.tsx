"use client";

import { useState } from "react";
import { IconAlertTriangle, IconCircleCheck, IconRefresh } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import type { ProviderHealth } from "@/lib/state/slices/office/types";
import { providerLabel } from "./provider-order-editor";
import { useTranslation } from "react-i18next";

type Props = {
  health: ProviderHealth[];
  onRetry: (providerId: string) => Promise<void>;
};

// Catalog keys, not copy — module scope freezes a `t()` at the boot locale. The
// record keys are the wire `ProviderHealth["state"]` values.
const STATE_BADGE: Record<ProviderHealth["state"], { labelKey: string; variant: BadgeVariant }> = {
  healthy: { labelKey: "office:healthy", variant: "outline" },
  short_retry: { labelKey: "office:shortRetry", variant: "outline" },
  degraded: { labelKey: "office:degraded", variant: "destructive" },
  user_action_required: { labelKey: "office:needsAction", variant: "destructive" },
};

type BadgeVariant = "default" | "secondary" | "destructive" | "outline";

export function ProviderHealthBanner({ health, onRetry }: Props) {
  const { t } = useTranslation();
  const nonHealthy = health.filter((h) => h.state !== "healthy");
  if (nonHealthy.length === 0) {
    return (
      <div className="rounded-lg border border-border p-3 flex items-center gap-2 text-sm">
        <IconCircleCheck className="h-4 w-4 text-emerald-600" />
        <span>{t("office:allProvidersHealthy")}</span>
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
  const { t } = useTranslation();
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
          <Badge variant={meta.variant}>{t(meta.labelKey)}</Badge>
          {h.error_code && (
            <span className="text-xs font-mono text-muted-foreground">{h.error_code}</span>
          )}
        </div>
        {/* `{{when}}` is a raw backend timestamp, not copy. */}
        {h.retry_at && (
          <p className="text-xs text-muted-foreground">
            {t("office:retryAtWhen", { when: h.retry_at })}
          </p>
        )}
      </div>
      <Button
        size="sm"
        variant="outline"
        onClick={handleRetry}
        disabled={busy}
        className="cursor-pointer gap-1"
      >
        <IconRefresh className="h-3.5 w-3.5" />
        {busy ? t("office:retrying") : t("office:retry")}
      </Button>
    </div>
  );
}
