"use client";

import { useState } from "react";
import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { useAutoUpdateSettings } from "@/hooks/domains/plugins/use-auto-update-settings";
import { usePlugins } from "@/hooks/domains/plugins/use-plugins";
import { usePluginUpdates } from "@/hooks/domains/plugins/use-plugin-updates";
import type { MarketplaceEntry } from "@/lib/types/plugins";
import { InstallPluginDialog } from "./install-plugin-dialog";
import { MarketplaceBrowser } from "./marketplace-browser";
import { PluginRow } from "./plugin-row";
import { UninstallPluginDialog } from "./uninstall-plugin-dialog";
import { usePluginActions } from "./use-plugin-actions";

/**
 * Operator UI to browse, install, enable, disable, uninstall, and update kandev
 * plugins (docs/specs/plugins/marketplace.md). Gated on the `plugins` feature
 * flag by the page-level default export.
 */
export function PluginsSettings() {
  const list = usePlugins();
  const actions = usePluginActions();
  const autoUpdate = useAutoUpdateSettings();
  const { updates, reload: reloadUpdates } = usePluginUpdates();
  const [updatingId, setUpdatingId] = useState<string | null>(null);

  // Update = install the newer package over the current one (marketplaceInstall
  // upserts the new record into the store, so the row's version refreshes),
  // then re-check the catalog so the resolved update drops off the row.
  const handleUpdate = async (entry: MarketplaceEntry) => {
    setUpdatingId(entry.id);
    try {
      await actions.marketplaceInstall(entry.package_url);
      reloadUpdates();
    } finally {
      setUpdatingId(null);
    }
  };

  return (
    <SettingsPageTemplate
      title="Plugins"
      description="Browse the marketplace, then install, enable, disable, and uninstall kandev plugins."
      isDirty={false}
      saveStatus="idle"
      onSave={() => undefined}
      showSaveButton={false}
    >
      <Tabs defaultValue="installed" className="space-y-6">
        <TabsList>
          <TabsTrigger
            value="installed"
            data-testid="plugins-tab-installed"
            className="cursor-pointer"
          >
            Installed
          </TabsTrigger>
          <TabsTrigger value="browse" data-testid="plugins-tab-browse" className="cursor-pointer">
            Browse
          </TabsTrigger>
        </TabsList>

        <TabsContent value="installed" className="space-y-6">
          <InstalledTab
            list={list}
            actions={actions}
            autoUpdate={autoUpdate}
            updates={updates}
            updatingId={updatingId}
            onUpdate={handleUpdate}
          />
        </TabsContent>

        <TabsContent value="browse">
          <MarketplaceBrowser onInstallUrl={actions.marketplaceInstall} />
        </TabsContent>
      </Tabs>

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

type InstalledTabProps = {
  list: ReturnType<typeof usePlugins>;
  actions: ReturnType<typeof usePluginActions>;
  autoUpdate: ReturnType<typeof useAutoUpdateSettings>;
  updates: Map<string, MarketplaceEntry>;
  updatingId: string | null;
  onUpdate: (entry: MarketplaceEntry) => void;
};

/** The Installed tab: auto-update toggle, sync/install toolbar, sync errors, and the plugin list. */
function InstalledTab({
  list,
  actions,
  autoUpdate,
  updates,
  updatingId,
  onUpdate,
}: InstalledTabProps) {
  return (
    <>
      <GlobalAutoUpdateToggle settings={autoUpdate} />

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
        list={list}
        actions={actions}
        autoUpdateDefault={autoUpdate.autoUpdateDefault}
        updates={updates}
        updatingId={updatingId}
        onUpdate={onUpdate}
      />
    </>
  );
}

/**
 * The instance-wide "Automatically update plugins" switch. When on, every
 * installed plugin without its own per-row override is auto-updated in the
 * background. Individual rows can still override this either way.
 */
function GlobalAutoUpdateToggle({
  settings,
}: {
  settings: ReturnType<typeof useAutoUpdateSettings>;
}) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-lg border border-border/70 bg-background p-4">
      <div className="min-w-0 space-y-1">
        <label
          htmlFor="plugins-auto-update-default"
          className="text-sm font-medium text-foreground cursor-pointer"
        >
          Automatically update plugins
        </label>
        <p className="text-xs text-muted-foreground">
          Check the marketplace periodically and install newer versions of active plugins. Applies
          to every plugin unless overridden per plugin below.
        </p>
      </div>
      <Switch
        id="plugins-auto-update-default"
        data-testid="plugins-auto-update-default"
        checked={settings.autoUpdateDefault}
        disabled={!settings.loaded}
        onCheckedChange={settings.setDefault}
        className="cursor-pointer"
      />
    </div>
  );
}

type PluginListProps = {
  list: ReturnType<typeof usePlugins>;
  actions: ReturnType<typeof usePluginActions>;
  autoUpdateDefault: boolean;
  updates: Map<string, MarketplaceEntry>;
  updatingId: string | null;
  onUpdate: (entry: MarketplaceEntry) => void;
};

function PluginList({
  list,
  actions,
  autoUpdateDefault,
  updates,
  updatingId,
  onUpdate,
}: PluginListProps) {
  const { items, loaded, loading, error } = list;

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
        No plugins yet. Install one from the Browse tab, or upload a package.
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {items.map((plugin) => (
        <PluginRow
          key={plugin.id}
          plugin={plugin}
          busy={actions.busyId === plugin.id || updatingId === plugin.id}
          update={updates.get(plugin.id)}
          autoUpdateDefault={autoUpdateDefault}
          autoUpdateBusy={actions.autoUpdateBusyId === plugin.id}
          onEnable={actions.handleEnable}
          onDisable={actions.handleDisable}
          onUninstall={actions.openUninstall}
          onUpdate={onUpdate}
          onSetAutoUpdate={actions.handleSetAutoUpdate}
        />
      ))}
    </div>
  );
}
