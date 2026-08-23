"use client";

import { useRef, useState, type RefObject } from "react";
import {
  IconArrowUpCircle,
  IconChevronRight,
  IconLoader2,
  IconSettings,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import Link from "@/components/routing/app-link";
import { PluginRepoLink } from "./plugin-repo-link";
import { PluginStatusBadge } from "./plugin-status-badge";
import { PluginErrorDiagnostic } from "./plugin-error-diagnostic";
import { PluginUninstallConfirmation } from "./uninstall-plugin-dialog";
import type { MarketplaceEntry, PluginRecord } from "@/lib/types/plugins";
import { SETTINGS_TYPOGRAPHY } from "@/components/settings/settings-typography";

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
  /**
   * True when the last check reached some sources but not all of them. A
   * plugin absent from a partial catalog is unknown, not delisted, so the
   * not-in-marketplace hint is withheld.
   */
  sourcesDegraded?: boolean;
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
  /** True when the plugin declares required settings the operator has not filled in. */
  needsSetup?: boolean;
  /** Fine pointers use an anchored popover; coarse pointers use inline row actions. */
  isFinePointer?: boolean;
  /** True while this plugin's uninstall request is in flight. */
  uninstallBusy?: boolean;
  onEnable: (plugin: PluginRecord) => void;
  onDisable: (plugin: PluginRecord) => void;
  onConfirmUninstall?: (plugin: PluginRecord) => void | Promise<void>;
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
  isFinePointer = true,
  uninstallBusy = false,
  onEnable,
  onDisable,
  onConfirmUninstall,
  onUpdate,
  onSetAutoUpdate,
}: PluginRowProps) {
  const canEnable =
    plugin.status === "disabled" || plugin.status === "registered" || plugin.status === "error";
  const canDisable = plugin.status === "active" || plugin.status === "error";
  const mutationBusy = busy || autoUpdateBusy || uninstallBusy;
  const {
    confirmingUninstall,
    setConfirmingUninstall,
    uninstallAnchorRef,
    requestUninstall,
    cancelUninstall,
    confirmUninstall,
  } = usePluginUninstallConfirmation(plugin, onConfirmUninstall);

  return (
    <div
      data-testid={`plugin-row-${plugin.id}`}
      className="group relative rounded-lg border border-border/70 bg-background p-4 transition-colors hover:border-border hover:bg-muted"
    >
      <PluginRowContent
        plugin={plugin}
        update={update}
        autoUpdateDefault={autoUpdateDefault}
        needsSetup={needsSetup}
        isFinePointer={isFinePointer}
        mutationBusy={mutationBusy}
        canEnable={canEnable}
        canDisable={canDisable}
        confirmingUninstall={confirmingUninstall}
        uninstallAnchorRef={uninstallAnchorRef}
        onEnable={onEnable}
        onDisable={onDisable}
        onUninstall={requestUninstall}
        onUpdate={onUpdate}
        onSetAutoUpdate={onSetAutoUpdate}
      />
      <div className="relative z-10">
        <PluginUninstallConfirmation
          target={plugin}
          open={confirmingUninstall}
          isFinePointer={isFinePointer}
          anchorRef={uninstallAnchorRef}
          onOpenChange={setConfirmingUninstall}
          onCancel={cancelUninstall}
          onConfirm={confirmUninstall}
        />
      </div>
    </div>
  );
}

type PluginRowContentProps = {
  plugin: PluginRecord;
  update?: PluginRowUpdateState;
  autoUpdateDefault: boolean;
  needsSetup: boolean;
  isFinePointer: boolean;
  mutationBusy: boolean;
  canEnable: boolean;
  canDisable: boolean;
  confirmingUninstall: boolean;
  uninstallAnchorRef: RefObject<HTMLButtonElement | null>;
  onEnable: (plugin: PluginRecord) => void;
  onDisable: (plugin: PluginRecord) => void;
  onUninstall: (plugin: PluginRecord) => void;
  onUpdate?: (entry: MarketplaceEntry) => void;
  onSetAutoUpdate: (plugin: PluginRecord, value: boolean | null) => void;
};

function PluginRowContent({
  plugin,
  update,
  autoUpdateDefault,
  needsSetup,
  isFinePointer,
  mutationBusy,
  canEnable,
  canDisable,
  confirmingUninstall,
  uninstallAnchorRef,
  onEnable,
  onDisable,
  onUninstall,
  onUpdate,
  onSetAutoUpdate,
}: PluginRowContentProps) {
  const { t } = useTranslation();

  return (
    <>
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
          <PluginRowIdentity plugin={plugin} needsSetup={needsSetup} update={update} />

          <div className="flex items-center gap-2 shrink-0">
            <PluginRowActions
              plugin={plugin}
              busy={mutationBusy}
              update={update}
              canEnable={canEnable}
              canDisable={canDisable}
              isFinePointer={isFinePointer}
              confirmingUninstall={confirmingUninstall}
              uninstallAnchorRef={uninstallAnchorRef}
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
              <Badge key={category} variant="secondary" className={SETTINGS_TYPOGRAPHY.meta}>
                {category}
              </Badge>
            ))}
          </div>
        )}

        <PluginAutoUpdateRow
          plugin={plugin}
          autoUpdateDefault={autoUpdateDefault}
          busy={mutationBusy}
          onSetAutoUpdate={onSetAutoUpdate}
        />
      </div>
    </>
  );
}

function usePluginUninstallConfirmation(
  plugin: PluginRecord,
  onConfirmUninstall?: (plugin: PluginRecord) => void | Promise<void>,
) {
  const [confirmingUninstall, setConfirmingUninstall] = useState(false);
  const uninstallAnchorRef = useRef<HTMLButtonElement>(null);

  const requestUninstall = () => {
    setConfirmingUninstall(true);
  };

  const cancelUninstall = () => {
    setConfirmingUninstall(false);
  };

  const confirmUninstall = () => {
    setConfirmingUninstall(false);
    return onConfirmUninstall?.(plugin);
  };

  return {
    confirmingUninstall,
    setConfirmingUninstall,
    uninstallAnchorRef,
    requestUninstall,
    cancelUninstall,
    confirmUninstall,
  };
}

/**
 * The row's name, state badges and identifiers. Everything here is inert and
 * sits under the card's overlay link, except the repo link, which raises
 * itself to z-10 to keep its own click.
 */
function PluginRowIdentity({
  plugin,
  needsSetup,
  update,
}: {
  plugin: PluginRecord;
  needsSetup: boolean;
  update?: PluginRowUpdateState;
}) {
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
            className={"border-primary/40 bg-primary/10 text-primary " + SETTINGS_TYPOGRAPHY.meta}
          >
            {t("plugins:setupRequired")}
          </Badge>
        )}
        {plugin.signed === false && (
          <Badge
            data-testid="plugin-unsigned-badge"
            variant="outline"
            className={
              "border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400 " +
              SETTINGS_TYPOGRAPHY.meta
            }
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
        <PluginUpdateInfo pluginId={plugin.id} update={update} />
      </div>
    </div>
  );
}

/**
 * The latest-known marketplace version for this plugin, once a catalog check
 * has completed at least once: "Latest v<x>" or a not-in-marketplace hint
 * when no source carries this plugin id at all. The update action itself is
 * the update-available affordance, so the row does not repeat it as a badge.
 * Renders nothing before the first successful check — a stale "not in
 * marketplace" flash would be actively misleading while the marketplace
 * hasn't been queried yet — and likewise nothing when the check only reached
 * some sources, since a plugin carried solely by the source that failed is
 * unknown, not delisted.
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
    if (update.sourcesDegraded) return null;
    return (
      <span data-testid={`plugin-not-in-marketplace-${pluginId}`}>
        {t("plugins:notInMarketplace")}
      </span>
    );
  }

  return (
    <span data-testid={`plugin-latest-version-${pluginId}`}>
      {t("plugins:latestVersion", { version: update.latest.version })}
    </span>
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
          <Badge variant="outline" className={SETTINGS_TYPOGRAPHY.meta}>
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
  | "onConfirmUninstall"
  | "uninstallBusy"
> & {
  canEnable: boolean;
  canDisable: boolean;
  isFinePointer: boolean;
  confirmingUninstall: boolean;
  uninstallAnchorRef: RefObject<HTMLButtonElement | null>;
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
  isFinePointer,
  confirmingUninstall,
  uninstallAnchorRef,
  onEnable,
  onDisable,
  onUninstall,
  onUpdate,
}: PluginRowActionsProps) {
  const { t } = useTranslation();
  const updateEntry = update?.hasUpdate ? update.latest : undefined;
  return (
    <div className="relative z-10 flex flex-wrap items-center gap-2 shrink-0">
      {updateEntry && onUpdate && (
        <Button
          variant="default"
          size="sm"
          data-testid={`plugin-update-${plugin.id}`}
          className="cursor-pointer gap-1 min-h-11 sm:min-h-0"
          aria-busy={update?.busy ? "true" : undefined}
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
      {(isFinePointer || !confirmingUninstall) && (
        <Button
          ref={uninstallAnchorRef}
          variant="ghost"
          size="sm"
          className="cursor-pointer min-h-11 text-destructive hover:text-destructive sm:min-h-0"
          disabled={busy}
          onClick={() => onUninstall(plugin)}
        >
          {t("plugins:uninstall")}
        </Button>
      )}
      <Link
        href={`/settings/plugins/${encodeURIComponent(plugin.id)}`}
        data-testid={`plugin-settings-link-${plugin.id}`}
        aria-label={t("plugins:openSettingsFor", { name: plugin.display_name })}
        className="inline-flex min-h-11 shrink-0 items-center gap-1 rounded-md px-2 text-xs text-muted-foreground hover:bg-muted hover:text-foreground cursor-pointer sm:min-h-0"
      >
        <IconSettings className="h-4 w-4" aria-hidden />
        {t("plugins:settings")}
      </Link>
    </div>
  );
}
