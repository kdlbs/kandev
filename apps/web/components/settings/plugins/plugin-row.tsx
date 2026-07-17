"use client";

import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { PluginStatusBadge } from "./plugin-status-badge";
import type { PluginRecord } from "@/lib/types/plugins";

type PluginRowProps = {
  plugin: PluginRecord;
  busy: boolean;
  onEnable: (plugin: PluginRecord) => void;
  onDisable: (plugin: PluginRecord) => void;
  onUninstall: (plugin: PluginRecord) => void;
};

/**
 * One plugin's row. Div-based (not a `<table>`) so it wraps/stacks naturally
 * on narrow viewports and inside the mobile settings sheet — no separate
 * mobile layout needed.
 */
export function PluginRow({ plugin, busy, onEnable, onDisable, onUninstall }: PluginRowProps) {
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
            <span className="text-sm font-medium text-foreground truncate">
              {plugin.display_name}
            </span>
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
          <div className="text-xs text-muted-foreground font-mono truncate">
            {plugin.id} · v{plugin.version}
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

        <div className="flex flex-wrap items-center gap-2 shrink-0">
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
      </div>
    </div>
  );
}
