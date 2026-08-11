"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { useAutoUpdateSettings } from "@/hooks/domains/plugins/use-auto-update-settings";
import { usePlugins } from "@/hooks/domains/plugins/use-plugins";
import { usePluginUpdates } from "@/hooks/domains/plugins/use-plugin-updates";
import { InstallPluginDialog } from "./install-plugin-dialog";
import { MarketplaceBrowser } from "./marketplace-browser";
import { PluginRow, type PluginRowUpdateState } from "./plugin-row";
import { PluginUpdateStatus } from "./plugin-update-status";
import { UninstallPluginDialog } from "./uninstall-plugin-dialog";
import { usePluginActions } from "./use-plugin-actions";
import { usePluginUpdateAction } from "./use-plugin-update-action";

/**
 * Operator UI to browse, install, enable, disable, uninstall, and update kandev
 * plugins (docs/specs/plugins/marketplace.md). Gated on the `plugins` feature
 * flag by the page-level default export.
 */
export function PluginsSettings() {
  const { t } = useTranslation();
  const list = usePlugins();
  const actions = usePluginActions();
  const autoUpdate = useAutoUpdateSettings();
  const updates = usePluginUpdates();
  const installedIds = useMemo(() => new Set(list.items.map((p) => p.id)), [list.items]);
  const updateAction = usePluginUpdateAction(
    actions.marketplaceInstall,
    updates.reload,
    installedIds,
  );

  // Sync scans the local plugins folder; it never touches the marketplace on
  // its own. Chaining a catalog check after it is what makes the button's
  // result (newer versions) actually show up. Skip the check when the sync
  // itself failed against an unreachable backend — a second failure on the
  // same root cause would just be a redundant error.
  const handleSyncAndCheck = async () => {
    const result = await actions.handleSync();
    if (result.ok) {
      await updates.checkForUpdates();
    }
  };

  return (
    <SettingsPageTemplate
      title={t("common:plugins")}
      description={t("plugins:settingsDescription")}
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
            {t("plugins:tabInstalled")}
          </TabsTrigger>
          <TabsTrigger value="browse" data-testid="plugins-tab-browse" className="cursor-pointer">
            {t("plugins:tabBrowse")}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="installed" className="space-y-6">
          <InstalledTab
            list={list}
            actions={actions}
            autoUpdate={autoUpdate}
            updates={updates}
            updateAction={updateAction}
            onSync={handleSyncAndCheck}
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
  updates: ReturnType<typeof usePluginUpdates>;
  updateAction: ReturnType<typeof usePluginUpdateAction>;
  onSync: () => void;
};

/** The Installed tab: auto-update toggle, sync/install toolbar, update status, sync errors, and the plugin list. */
function InstalledTab({
  list,
  actions,
  autoUpdate,
  updates,
  updateAction,
  onSync,
}: InstalledTabProps) {
  const { t } = useTranslation();
  const syncing = actions.syncBusy || updates.checking;

  return (
    <>
      <GlobalAutoUpdateToggle settings={autoUpdate} />

      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="text-sm font-medium text-foreground">{t("plugins:installedPlugins")}</div>
        <div className="flex items-center gap-2">
          <Button
            data-testid="plugins-sync-button"
            variant="secondary"
            disabled={syncing}
            onClick={onSync}
            className="cursor-pointer"
          >
            <IconRefresh className={`h-4 w-4 ${syncing ? "animate-spin" : ""}`} />
            {t("plugins:sync")}
          </Button>
          <Button
            data-testid="install-plugin-trigger"
            onClick={actions.openInstall}
            className="cursor-pointer"
          >
            {t("plugins:installPlugin")}
          </Button>
        </div>
      </div>

      <PluginUpdateStatus
        checking={updates.checking}
        lastCheckedAt={updates.lastCheckedAt}
        error={updates.error}
      />

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
        updateAction={updateAction}
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
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-4 rounded-lg border border-border/70 bg-background p-4">
      <div className="min-w-0 space-y-1">
        <label
          htmlFor="plugins-auto-update-default"
          className="text-sm font-medium text-foreground cursor-pointer"
        >
          {t("plugins:autoUpdateTitle")}
        </label>
        <p className="text-xs text-muted-foreground">{t("plugins:autoUpdateDescription")}</p>
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
  updates: ReturnType<typeof usePluginUpdates>;
  updateAction: ReturnType<typeof usePluginUpdateAction>;
};

function PluginList({ list, actions, autoUpdateDefault, updates, updateAction }: PluginListProps) {
  const { t } = useTranslation();
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
        {t("plugins:loadingPlugins")}
      </div>
    );
  }

  if (loaded && items.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
        {t("plugins:noPluginsYet")}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {items.map((plugin) => {
        const rowUpdate: PluginRowUpdateState = {
          latest: updates.latestById.get(plugin.id),
          hasUpdate: updates.updates.has(plugin.id),
          checked: updates.checked,
          sourcesDegraded: updates.sourcesDegraded,
          busy: updateAction.updatingIds.has(plugin.id),
          error: updateAction.errorsById.get(plugin.id),
        };
        return (
          <PluginRow
            key={plugin.id}
            plugin={plugin}
            busy={actions.busyId === plugin.id || rowUpdate.busy}
            update={rowUpdate}
            autoUpdateDefault={autoUpdateDefault}
            autoUpdateBusy={actions.autoUpdateBusyId === plugin.id}
            onEnable={actions.handleEnable}
            onDisable={actions.handleDisable}
            onUninstall={actions.openUninstall}
            onUpdate={updateAction.runUpdate}
            onSetAutoUpdate={actions.handleSetAutoUpdate}
          />
        );
      })}
    </div>
  );
}
