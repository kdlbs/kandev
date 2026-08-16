"use client";

import { IconArrowUpCircle, IconChevronRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import Link from "@/components/routing/app-link";
import { PluginRepoLink } from "./plugin-repo-link";
import { PluginStatusBadge } from "./plugin-status-badge";
import { PluginErrorDiagnostic } from "./plugin-error-diagnostic";
import type { MarketplaceEntry, PluginRecord } from "@/lib/types/plugins";

type PluginRowProps = {
  plugin: PluginRecord;
  busy: boolean;
  /** Set when the marketplace has a newer version than the installed one. */
  update?: MarketplaceEntry;
  /** The instance-wide auto-update default, used when the plugin has no override. */
  autoUpdateDefault: boolean;
  /** True while this row's auto-update override request is in flight. */
  autoUpdateBusy: boolean;
  /** True when the plugin declares required settings the operator has not filled in. */
  needsSetup?: boolean;
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
 *
 * The whole card opens the plugin's settings page, via an overlay link and a
 * trailing chevron (the pattern the workspaces list already uses). The name
 * alone used to carry the link, which was indistinguishable from a heading
 * until hovered — and therefore invisible on touch, where nothing hovers.
 * Every control in the card sits above the overlay at z-10 so it keeps its
 * own click.
 */
export function PluginRow({
  plugin,
  busy,
  update,
  autoUpdateDefault,
  autoUpdateBusy,
  needsSetup = false,
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
      className="group relative rounded-lg border border-border/70 bg-background p-4 transition-colors hover:border-border hover:bg-muted"
    >
      <Link
        href={`/settings/plugins/${encodeURIComponent(plugin.id)}`}
        aria-label={t("plugins:openSettingsFor", { name: plugin.display_name })}
        data-testid={`plugin-row-link-${plugin.id}`}
        className="absolute inset-0 rounded-lg cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      {/*
        The spacing lives on this inner wrapper, not on the card. `space-y-*`
        compiles to a margin on every child bar the last, and with the overlay
        link as a card child that margin shortened it by one gap, leaving the
        bottom strip of the card unclickable.
      */}
      <div className="space-y-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <PluginRowIdentity plugin={plugin} needsSetup={needsSetup} />

          <div className="flex items-center gap-2 shrink-0">
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
            <IconChevronRight
              aria-hidden
              className="h-5 w-5 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5"
            />
          </div>
        </div>

        {plugin.description && (
          <div className="text-xs text-muted-foreground">{plugin.description}</div>
        )}
        <PluginErrorDiagnostic plugin={plugin} />
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
    </div>
  );
}

/**
 * The row's name, state badges and identifiers. Everything here is inert and
 * sits under the card's overlay link, except the repo link, which raises
 * itself to z-10 to keep its own click.
 */
function PluginRowIdentity({ plugin, needsSetup }: { plugin: PluginRecord; needsSetup: boolean }) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 space-y-1">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium text-foreground truncate group-hover:underline">
          {plugin.display_name}
        </span>
        <PluginStatusBadge status={plugin.status} />
        {needsSetup && (
          <Badge
            data-testid={`plugin-setup-required-${plugin.id}`}
            variant="outline"
            className="border-primary/40 bg-primary/10 text-primary text-[11px]"
          >
            {t("plugins:setupRequired")}
          </Badge>
        )}
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
        <PluginRepoLink url={plugin.repo_url} className="relative z-10" />
      </div>
    </div>
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
      <div className="relative z-10 flex items-center gap-2">
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
  | "needsSetup"
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
  return (
    <div className="relative z-10 flex flex-wrap items-center gap-2 shrink-0">
      {update && onUpdate && (
        <Button
          variant="outline"
          size="sm"
          data-testid={`plugin-update-${plugin.id}`}
          className="cursor-pointer gap-1 min-h-11 sm:min-h-0"
          disabled={busy}
          onClick={() => onUpdate(update)}
        >
          <IconArrowUpCircle className="h-4 w-4" />
          {busy ? t("plugins:updating") : t("plugins:updateToVersion", { version: update.version })}
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
