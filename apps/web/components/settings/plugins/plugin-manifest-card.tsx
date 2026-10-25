"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { formatDateTime } from "@/lib/i18n/formats";
import type { PluginRecord } from "@/lib/types/plugins";
import { SETTINGS_TYPOGRAPHY } from "@/components/settings/settings-typography";
import { isPublicWebhook } from "./plugin-manifest-model";

/**
 * Read-only view of the plugin's manifest: identity, capabilities, declared
 * webhooks, and a collapsible raw JSON dump for anything the summary
 * rows don't surface.
 */
export function PluginManifestCard({ plugin }: { plugin: PluginRecord }) {
  const { t } = useTranslation();
  const [showRaw, setShowRaw] = useState(false);

  return (
    <Card data-testid="plugin-manifest-card">
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle className="text-base">{t("plugins:manifest")}</CardTitle>
        <Button
          variant="ghost"
          size="sm"
          className="cursor-pointer"
          onClick={() => setShowRaw((v) => !v)}
        >
          {showRaw ? t("plugins:hideRaw") : t("plugins:viewRaw")}
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-2 text-sm">
          {/* Row VALUES are manifest data — the plugin id, its version, its
              author — and are never translated. Only the row labels are copy. */}
          <ManifestRow label={t("plugins:manifestId")} value={plugin.id} mono />
          <ManifestRow label={t("plugins:manifestVersion")} value={plugin.version} mono />
          <ManifestRow label={t("plugins:manifestApiVersion")} value={String(plugin.api_version)} />
          <ManifestRow label={t("plugins:manifestAuthor")} value={plugin.author || "-"} />
          <ManifestRow
            label={t("plugins:manifestSigned")}
            value={plugin.signed ? t("plugins:yes") : t("plugins:no")}
          />
          <ManifestRow
            label={t("plugins:manifestInstalled")}
            value={formatInstalledAt(plugin.installed_at)}
          />
        </div>

        <CapabilityBadges plugin={plugin} />
        <DeclarationList
          label={t("plugins:manifestWebhooks")}
          items={(plugin.webhooks ?? []).map((w) => ({
            key: w.key,
            text: isPublicWebhook(plugin.api_version, w.access)
              ? t("plugins:manifestWebhookPublic", { key: w.key })
              : w.key,
          }))}
        />

        {showRaw && (
          <pre
            data-testid="plugin-manifest-raw"
            className="rounded-md border border-border/70 bg-muted/40 p-3 text-xs overflow-x-auto"
          >
            {JSON.stringify(manifestOnly(plugin), null, 2)}
          </pre>
        )}
      </CardContent>
    </Card>
  );
}

function ManifestRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-baseline justify-between gap-3 sm:justify-start">
      <span className="text-muted-foreground w-28 shrink-0">{label}</span>
      <span className={mono ? "font-mono text-xs truncate" : "truncate"}>{value}</span>
    </div>
  );
}

function CapabilityBadges({ plugin }: { plugin: PluginRecord }) {
  const { t } = useTranslation();
  const caps = plugin.capabilities ?? {};
  const badges: string[] = [
    ...(caps.events ?? []).map((e) => `events:${e}`),
    ...(caps.api_read ?? []).map((r) => `read:${r}`),
    ...(caps.api_write ?? []).map((w) => `write:${w}`),
    ...(caps.state ? ["state"] : []),
    ...(caps.secrets ? ["secrets"] : []),
  ];
  if (badges.length === 0) return null;
  return (
    <div className="space-y-1.5">
      {/* The badges are permission scopes (`events:*`, `read:*`) — the
          contract the plugin declared, so they stay verbatim. */}
      <div className="text-sm text-muted-foreground">{t("plugins:manifestCapabilities")}</div>
      <div className="flex flex-wrap gap-1">
        {badges.map((badge) => (
          <Badge
            key={badge}
            variant="secondary"
            className={SETTINGS_TYPOGRAPHY.meta + " font-mono"}
          >
            {badge}
          </Badge>
        ))}
      </div>
    </div>
  );
}

function DeclarationList({
  label,
  items,
}: {
  label: string;
  items: { key: string; text: string }[];
}) {
  if (items.length === 0) return null;
  return (
    <div className="space-y-1.5">
      <div className="text-sm text-muted-foreground">{label}</div>
      <div className="flex flex-wrap gap-1">
        {items.map((item) => (
          <Badge key={item.key} variant="outline" className={SETTINGS_TYPOGRAPHY.meta}>
            {item.text}
          </Badge>
        ))}
      </div>
    </div>
  );
}

/** The manifest-declared fields only — runtime bookkeeping (status, install
 * path, restart counter, and failure diagnostic) is shown elsewhere and just
 * noise here. */
function manifestOnly(plugin: PluginRecord): Record<string, unknown> {
  const {
    status: _s,
    install_path: _i,
    restart_count: _r,
    installed_at: _a,
    last_error: _e,
    last_error_at: _ea,
    ...manifest
  } = plugin;
  return manifest;
}

function formatInstalledAt(installedAt: string): string {
  const date = new Date(installedAt);
  return Number.isNaN(date.getTime()) ? installedAt : formatDateTime(date);
}
