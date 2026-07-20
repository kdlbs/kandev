"use client";

import { IconArrowUpCircle } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { PluginRepoLink } from "./plugin-repo-link";
import { PluginStatusBadge } from "./plugin-status-badge";
import type { MarketplaceEntry, PluginRecord } from "@/lib/types/plugins";

type PluginRowProps = {
  plugin: PluginRecord;
  busy: boolean;
  /** Set when the marketplace has a newer version than the installed one. */
  update?: MarketplaceEntry;
  onEnable: (plugin: PluginRecord) => void;
  onDisable: (plugin: PluginRecord) => void;
  onUninstall: (plugin: PluginRecord) => void;
  onUpdate?: (entry: MarketplaceEntry) => void;
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
  onEnable,
  onDisable,
  onUninstall,
  onUpdate,
}: PluginRowProps) {
  const canEnable = plugin.status === "disabled" || plugin.status === "registered";
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
                unsigned
              </Badge>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span className="font-mono truncate">
              {plugin.id} · v{plugin.version}
            </span>
            {plugin.repo_url && <PluginRepoLink url={plugin.repo_url} />}
          </div>
          {plugin.description && (
            <div className="text-xs text-muted-foreground">{plugin.description}</div>
          )}
          {plugin.categories.length > 0 && (
            <div className="flex flex-wrap gap-1 pt-1">
              {plugin.categories.map((category) => (
                <Badge key={category} variant="secondary" className="text-[11px]">
                  {category}
                </Badge>
              ))}
            </div>
          )}
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
    </div>
  );
}

type PluginRowActionsProps = Omit<PluginRowProps, "onEnable" | "onDisable" | "onUninstall"> & {
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
  return (
    <div className="flex flex-wrap items-center gap-2 shrink-0">
      {update && onUpdate && (
        <Button
          variant="outline"
          size="sm"
          data-testid={`plugin-update-${plugin.id}`}
          className="cursor-pointer gap-1"
          disabled={busy}
          onClick={() => onUpdate(update)}
        >
          <IconArrowUpCircle className="h-4 w-4" />
          {busy ? "Updating…" : `Update to v${update.version}`}
        </Button>
      )}
      {canEnable && (
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer"
          disabled={busy}
          onClick={() => onEnable(plugin)}
        >
          Enable
        </Button>
      )}
      {canDisable && (
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer"
          disabled={busy}
          onClick={() => onDisable(plugin)}
        >
          Disable
        </Button>
      )}
      <Button
        variant="ghost"
        size="sm"
        className="cursor-pointer text-destructive hover:text-destructive"
        disabled={busy}
        onClick={() => onUninstall(plugin)}
      >
        Uninstall
      </Button>
    </div>
  );
}
