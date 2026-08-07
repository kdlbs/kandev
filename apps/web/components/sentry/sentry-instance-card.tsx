"use client";

import { IconPencil, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  IntegrationAuthStatusBanner,
  type IntegrationAuthHealth,
} from "@/components/integrations/auth-status-banner";
import type { SentryConfig } from "@/lib/types/sentry";
import { useTranslation } from "react-i18next";

// configToHealth maps an instance's backend-recorded probe fields to the shared
// auth-status banner shape. Returns null (banner hidden) until a secret exists.
export function configToHealth(config: SentryConfig): IntegrationAuthHealth | null {
  if (!config.hasSecret) return null;
  if (!config.lastCheckedAt) return { ok: false, error: "", checkedAt: null };
  return {
    ok: !!config.lastOk,
    error: config.lastError ?? "",
    checkedAt: new Date(config.lastCheckedAt),
  };
}

type SentryInstanceCardProps = {
  instance: SentryConfig;
  onEdit: () => void;
  onDelete: () => void;
};

// SentryInstanceCard renders one saved instance: its name, URL, per-instance
// auth-health banner, and edit/delete actions.
export function SentryInstanceCard({ instance, onEdit, onDelete }: SentryInstanceCardProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3 rounded-md border p-4" data-testid="sentry-instance-card">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-medium truncate" data-testid="sentry-instance-name">
            {instance.name}
          </p>
          <p className="text-xs text-muted-foreground truncate">{instance.url}</p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={onEdit}
            aria-label={t("sentry:editInstanceAria", { name: instance.name })}
            className="cursor-pointer gap-1"
            data-testid="sentry-instance-edit-button"
          >
            <IconPencil className="h-3.5 w-3.5" />
            {t("sentry:edit")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="destructive"
            onClick={onDelete}
            aria-label={t("sentry:deleteInstanceAria", { name: instance.name })}
            className="cursor-pointer gap-1"
            data-testid="sentry-instance-delete-button"
          >
            <IconTrash className="h-3.5 w-3.5" />
            {t("sentry:delete")}
          </Button>
        </div>
      </div>
      <IntegrationAuthStatusBanner health={configToHealth(instance)} />
    </div>
  );
}
