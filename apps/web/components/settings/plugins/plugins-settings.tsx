"use client";

import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { usePlugins } from "@/hooks/domains/plugins/use-plugins";
import { InstallPluginDialog } from "./install-plugin-dialog";
import { PluginRow } from "./plugin-row";
import { UninstallPluginDialog } from "./uninstall-plugin-dialog";
import { usePluginActions } from "./use-plugin-actions";

/**
 * Operator UI to install/list/enable/disable/uninstall plugins
 * (docs/plans/plugins/task-20-mgmt-page.md, reworked for the install-based
 * flow in docs/plans/plugins/GRPC-CONTRACT.md §7). Gated on the `plugins`
 * feature flag by the page-level default export.
 */
export function PluginsSettings() {
  const { items, loaded, loading, error } = usePlugins();
  const actions = usePluginActions();

  return (
    <SettingsPageTemplate
      title="Plugins"
      description="Install, enable, disable, and uninstall kandev plugins."
      isDirty={false}
      saveStatus="idle"
      onSave={() => undefined}
      showSaveButton={false}
    >
      <div className="space-y-6">
        <div className="flex items-center justify-between gap-2">
          <div className="text-sm font-medium text-foreground">Installed plugins</div>
          <div className="flex items-center gap-2">
            <Button
              data-testid="plugins-sync-button"
              variant="secondary"
              disabled={actions.syncBusy}
              onClick={actions.handleSync}
              className="cursor-pointer"
            >
              <IconRefresh className={`h-4 w-4 ${actions.syncBusy ? "animate-spin" : ""}`} />
              Sync
            </Button>
            <Button
              data-testid="install-plugin-trigger"
              onClick={actions.openInstall}
              className="cursor-pointer"
            >
              Install plugin
            </Button>
          </div>
        </div>

        {actions.syncErrors.length > 0 && (
          <div
            data-testid="plugins-sync-errors"
            className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-4 text-sm text-amber-700 dark:text-amber-400 space-y-1"
          >
            {actions.syncErrors.map((err) => (
              <div key={err.path} className="font-mono text-xs">
                {err.path}: {err.reason}
              </div>
            ))}
          </div>
        )}

        <PluginList
          items={items}
          loaded={loaded}
          loading={loading}
          error={error}
          actions={actions}
        />
      </div>

      <UninstallPluginDialog
        target={actions.uninstallTarget}
        busy={actions.uninstallBusy}
        onClose={actions.closeUninstall}
        onConfirm={actions.confirmUninstall}
      />
      <InstallPluginDialog
        open={actions.installOpen}
        busy={actions.installBusy}
        error={actions.installError}
        onOpenChange={actions.setInstallOpen}
        onSubmitUrl={actions.submitInstallUrl}
        onSubmitFile={actions.submitInstallFile}
      />
    </SettingsPageTemplate>
  );
}

type PluginListProps = {
  items: ReturnType<typeof usePlugins>["items"];
  loaded: boolean;
  loading: boolean;
  error: string | null;
  actions: ReturnType<typeof usePluginActions>;
};

function PluginList({ items, loaded, loading, error, actions }: PluginListProps) {
  if (error) {
    return (
      <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-6 text-sm text-destructive">
        {error}
      </div>
    );
  }

  if (!loaded && loading) {
    return (
      <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
        Loading plugins...
      </div>
    );
  }

  if (loaded && items.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
        No plugins yet. Install a plugin package to get started.
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {items.map((plugin) => (
        <PluginRow
          key={plugin.id}
          plugin={plugin}
          busy={actions.busyId === plugin.id}
          onEnable={actions.handleEnable}
          onDisable={actions.handleDisable}
          onUninstall={actions.openUninstall}
        />
      ))}
    </div>
  );
}
