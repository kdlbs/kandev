"use client";

import { IconArrowUpCircle, IconLoader2 } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import Link from "@/components/routing/app-link";
import { PluginRepoLink } from "./plugin-repo-link";
import { PluginStatusBadge } from "./plugin-status-badge";
import { PluginErrorDiagnostic } from "./plugin-error-diagnostic";
import type { MarketplaceEntry, PluginRecord } from "@/lib/types/plugins";

/**
 * The row's view of its marketplace-update status, computed by
 * plugins-settings.tsx from usePluginUpdates + usePluginUpdateAction.
 * `checked` mirrors the hook's global flag (true once the first successful
 * catalog check has completed) so the row can distinguish "haven't checked
 * yet" from "checked and this plugin isn't in any catalog" — never flashing
 * a misleading "not in marketplace" before the first response arrives.
 */
export type PluginRowUpdateState = {
  /** This plugin's catalog entry, present once checked and found in any enabled source. */
  latest?: MarketplaceEntry;
  /** True when `latest` is strictly newer than the installed version. */
  hasUpdate: boolean;
  /** True once a successful catalog check has completed at least once. */
  checked: boolean;
  /** True while a manual update for this plugin is in flight. */
  busy: boolean;
  /** Set when the last manual update attempt for this plugin failed. */
  error?: string;
};

type PluginRowProps = {
  plugin: PluginRecord;
  busy: boolean;
  /** Marketplace version/update-check state for this plugin; absent when the parent has no update data at all. */
  update?: PluginRowUpdateState;
  /** The instance-wide auto-update default, used when the plugin has no override. */
  autoUpdateDefault: boolean;
  /** True while this row's auto-update override request is in flight. */
  autoUpdateBusy: boolean;
  onEnable: (plugin: PluginRecord) => void;
  onDisable: (plugin: PluginRecord) => void;
  onUninstall: (plugin: PluginRecord) => void;
  onUpdate?: (entry: MarketplaceEntry) => void;
  onSetAutoUpdate: (plugin: PluginRecord, value: boolean | null) => void;
};

/**
 * One plugin's row. Div-based (not a `<table>`) so it wraps/stacks naturally
 * on narrow viewports and inside the mobile settings sheet — no separate
 * mobile layout needed.
 */
export function PluginRow({
  plugin,
  busy,
  update,
  autoUpdateDefault,
  autoUpdateBusy,
  onEnable,
  onDisable,
  onUninstall,
  onUpdate,
  onSetAutoUpdate,
}: PluginRowProps) {
  const { t } = useTranslation();
  const canEnable =
    plugin.status === "disabled" || plugin.status === "registered" || plugin.status === "error";
  const canDisable = plugin.status === "active" || plugin.status === "error";

  return (
    <div
      data-testid={`plugin-row-${plugin.id}`}
      className="rounded-lg border border-border/70 bg-background p-4 space-y-3"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <Link
              href={`/settings/plugins/${encodeURIComponent(plugin.id)}`}
              data-testid={`plugin-row-link-${plugin.id}`}
              className="text-sm font-medium text-foreground truncate cursor-pointer hover:underline"
            >
              {plugin.display_name}
            </Link>
            <PluginStatusBadge status={plugin.status} />
            {plugin.signed === false && (
              <Badge
                data-testid="plugin-unsigned-badge"
                variant="outline"
                className="border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400 text-[11px]"
              >
                {t("plugins:unsigned")}
              </Badge>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span className="font-mono truncate">
              {plugin.id} · v{plugin.version}
            </span>
            <PluginRepoLink url={plugin.repo_url} />
            <PluginUpdateInfo pluginId={plugin.id} update={update} />
          </div>
        </div>

        <PluginRowActions
          plugin={plugin}
          busy={busy}
          update={update}
          canEnable={canEnable}
          canDisable={canDisable}
          onEnable={onEnable}
          onDisable={onDisable}
          onUninstall={onUninstall}
          onUpdate={onUpdate}
        />
      </div>

      {plugin.description && (
        <div className="text-xs text-muted-foreground">{plugin.description}</div>
      )}
      <PluginErrorDiagnostic plugin={plugin} />
      {update?.error && (
        <div
          role="alert"
          data-testid={`plugin-update-error-${plugin.id}`}
          className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive [overflow-wrap:anywhere]"
        >
          {update.error}
        </div>
      )}
      {plugin.categories.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {plugin.categories.map((category) => (
            <Badge key={category} variant="secondary" className="text-[11px]">
              {category}
            </Badge>
          ))}
        </div>
      )}

      <PluginAutoUpdateRow
        plugin={plugin}
        autoUpdateDefault={autoUpdateDefault}
        busy={autoUpdateBusy}
        onSetAutoUpdate={onSetAutoUpdate}
      />
    </div>
  );
}

/**
 * The latest-known marketplace version for this plugin, once a catalog check
 * has completed at least once: "Latest v<x>" plus an "Update available"
 * badge when it's newer than the installed version, or a not-in-marketplace
 * hint when no source carries this plugin id at all. Renders nothing before
 * the first successful check — a stale "not in marketplace" flash would be
 * actively misleading while the marketplace hasn't been queried yet.
 */
function PluginUpdateInfo({
  pluginId,
  update,
}: {
  pluginId: string;
  update?: PluginRowUpdateState;
}) {
  const { t } = useTranslation();
  if (!update?.checked) return null;

  if (!update.latest) {
    return (
      <span data-testid={`plugin-not-in-marketplace-${pluginId}`}>
        {t("plugins:notInMarketplace")}
      </span>
    );
  }

  return (
    <>
      <span data-testid={`plugin-latest-version-${pluginId}`}>
        {t("plugins:latestVersion", { version: update.latest.version })}
      </span>
      {update.hasUpdate && (
        <Badge
          data-testid={`plugin-update-available-${pluginId}`}
          variant="outline"
          className="border-blue-500/40 bg-blue-500/10 text-blue-600 dark:text-blue-400 text-[11px]"
        >
          {t("plugins:updateAvailableVersion", { version: update.latest.version })}
        </Badge>
      )}
    </>
  );
}

/**
 * The per-plugin auto-update control. The switch reflects the effective state
 * (the plugin's own override, or the instance-wide default when it has none);
 * toggling it sets an explicit override. Once overridden, a "Reset" affordance
 * clears the override so the plugin follows the global default again.
 */
function PluginAutoUpdateRow({
  plugin,
  autoUpdateDefault,
  busy,
  onSetAutoUpdate,
}: {
  plugin: PluginRecord;
  autoUpdateDefault: boolean;
  busy: boolean;
  onSetAutoUpdate: (plugin: PluginRecord, value: boolean | null) => void;
}) {
  const { t } = useTranslation();
  const isOverridden = plugin.auto_update !== null && plugin.auto_update !== undefined;
  const effective = isOverridden ? (plugin.auto_update as boolean) : autoUpdateDefault;

  return (
    <div className="flex items-center justify-between gap-3 border-t border-border/50 pt-3">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <span>{t("plugins:autoUpdate")}</span>
        {isOverridden && (
          <Badge variant="outline" className="text-[11px]">
            {t("plugins:override")}
          </Badge>
        )}
      </div>
      <div className="flex items-center gap-2">
        {isOverridden && (
          <button
            type="button"
            data-testid={`plugin-auto-update-reset-${plugin.id}`}
            aria-label={t("plugins:resetAutoUpdateFor", { name: plugin.display_name })}
            className="text-xs text-muted-foreground hover:text-foreground underline-offset-2 hover:underline cursor-pointer disabled:opacity-50"
            disabled={busy}
            onClick={() => onSetAutoUpdate(plugin, null)}
          >
            {t("plugins:reset")}
          </button>
        )}
        <Switch
          data-testid={`plugin-auto-update-${plugin.id}`}
          aria-label={t("plugins:autoUpdateFor", { name: plugin.display_name })}
          checked={effective}
          disabled={busy}
          onCheckedChange={(value) => onSetAutoUpdate(plugin, value)}
          className="cursor-pointer"
        />
      </div>
    </div>
  );
}

type PluginRowActionsProps = Omit<
  PluginRowProps,
  | "onEnable"
  | "onDisable"
  | "onUninstall"
  | "autoUpdateDefault"
  | "autoUpdateBusy"
  | "onSetAutoUpdate"
> & {
  canEnable: boolean;
  canDisable: boolean;
  onEnable: (plugin: PluginRecord) => void;
  onDisable: (plugin: PluginRecord) => void;
  onUninstall: (plugin: PluginRecord) => void;
};

function PluginRowActions({
  plugin,
  busy,
  update,
  canEnable,
  canDisable,
  onEnable,
  onDisable,
  onUninstall,
  onUpdate,
}: PluginRowActionsProps) {
  const { t } = useTranslation();
  const updateEntry = update?.hasUpdate ? update.latest : undefined;
  return (
    <div className="flex flex-wrap items-center gap-2 shrink-0">
      {updateEntry && onUpdate && (
        <Button
          variant="outline"
          size="sm"
          data-testid={`plugin-update-${plugin.id}`}
          className="cursor-pointer gap-1 min-h-11 sm:min-h-0"
          disabled={busy}
          onClick={() => onUpdate(updateEntry)}
        >
          {update?.busy ? (
            <IconLoader2 className="h-4 w-4 animate-spin" />
          ) : (
            <IconArrowUpCircle className="h-4 w-4" />
          )}
          {update?.busy
            ? t("plugins:updating")
            : t("plugins:updateToVersion", { version: updateEntry.version })}
        </Button>
      )}
      {canEnable && (
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer min-h-11 sm:min-h-0"
          disabled={busy}
          onClick={() => onEnable(plugin)}
        >
          {t("plugins:enable")}
        </Button>
      )}
      {canDisable && (
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer min-h-11 sm:min-h-0"
          disabled={busy}
          onClick={() => onDisable(plugin)}
        >
          {t("plugins:disable")}
        </Button>
      )}
      <Button
        variant="ghost"
        size="sm"
        className="cursor-pointer min-h-11 text-destructive hover:text-destructive sm:min-h-0"
        disabled={busy}
        onClick={() => onUninstall(plugin)}
      >
        {t("plugins:uninstall")}
      </Button>
    </div>
  );
}
