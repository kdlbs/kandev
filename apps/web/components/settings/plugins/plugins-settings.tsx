"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAutoUpdateSettings } from "@/hooks/domains/plugins/use-auto-update-settings";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { usePlugins } from "@/hooks/domains/plugins/use-plugins";
import { usePluginSetupStatus } from "@/hooks/domains/plugins/use-plugin-setup-status";
import { usePluginUpdates } from "@/hooks/domains/plugins/use-plugin-updates";
import { InstallPluginDialog } from "./install-plugin-dialog";
import { MarketplaceBrowser } from "./marketplace-browser";
import { CanvasMarketplace } from "./canvas-marketplace";
import { PluginRow, type PluginRowUpdateState } from "./plugin-row";
import { PluginUpdateStatus } from "./plugin-update-status";
import { usePluginActions } from "./use-plugin-actions";
import { usePluginUpdateAction } from "./use-plugin-update-action";
import { settingsActionClassName } from "@/components/settings/settings-control";
import type { MarketplaceEntry } from "@/lib/types/plugins";
import { SettingsRow } from "@/components/settings/settings-group";
import { SETTINGS_TYPOGRAPHY } from "@/components/settings/settings-typography";

/**
 * Operator UI to browse, install, enable, disable, uninstall, and update kandev
 * plugins (docs/specs/plugins/requirements/marketplace.md). Gated on the `plugins` feature
 * flag by the page-level default export.
 */
export function PluginsSettings() {
  const { t } = useTranslation();
  const canManage = useIsAdmin();
  const canvasesEnabled = useFeature("canvases");
  const { isFinePointer } = useResponsiveBreakpoint();
  const list = usePlugins();
  const actions = usePluginActions();
  const autoUpdate = useAutoUpdateSettings();
  const updates = usePluginUpdates();
  const installedIds = useMemo(() => new Set(list.items.map((p) => p.id)), [list.items]);
  const updateAction = usePluginUpdateAction(
    actions.marketplaceInstall,
    updates.reload,
    installedIds,
    updates.markUpdated,
  );

  const handleMarketplaceInstall = async (entry: MarketplaceEntry) => {
    const result = await actions.marketplaceInstall(entry);
    if (result.ok) await updates.reload(result.pluginId);
    return result;
  };

  return (
    <SettingsPageTemplate
      title={t("common:plugins")}
      description={t("plugins:settingsDescription")}
      isDirty={false}
      saveStatus="idle"
      onSave={() => undefined}
      showSaveButton={false}
      contentFrame="none"
    >
      <Tabs defaultValue="installed" className="space-y-4">
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
          {canvasesEnabled && (
            <TabsTrigger
              value="canvases"
              data-testid="plugins-tab-canvases"
              className="cursor-pointer"
            >
              {t("plugins:tabCanvases")}
            </TabsTrigger>
          )}
        </TabsList>

        <TabsContent value="installed" className="space-y-4">
          <InstalledTab
            list={list}
            actions={actions}
            autoUpdate={autoUpdate}
            updates={updates}
            canManage={canManage}
            updateAction={updateAction}
            isFinePointer={isFinePointer}
          />
        </TabsContent>

        <TabsContent value="browse">
          <MarketplaceBrowser onInstall={handleMarketplaceInstall} canManage={canManage} />
        </TabsContent>

        {canvasesEnabled && (
          <TabsContent value="canvases">
            <CanvasMarketplace />
          </TabsContent>
        )}
      </Tabs>

      {canManage && (
        <InstallPluginDialog
          open={actions.installOpen}
          busy={actions.installBusy}
          error={actions.installError}
          onOpenChange={actions.setInstallOpen}
          onSubmitUrl={actions.submitInstallUrl}
          onSubmitFile={actions.submitInstallFile}
        />
      )}
    </SettingsPageTemplate>
  );
}

type InstalledTabProps = {
  list: ReturnType<typeof usePlugins>;
  actions: ReturnType<typeof usePluginActions>;
  autoUpdate: ReturnType<typeof useAutoUpdateSettings>;
  canManage: boolean;
  updates: ReturnType<typeof usePluginUpdates>;
  updateAction: ReturnType<typeof usePluginUpdateAction>;
  isFinePointer: boolean;
};

/** The Installed tab: auto-update toggle, sync/install toolbar, update status, sync errors, and the plugin list. */
function InstalledTab({
  list,
  actions,
  autoUpdate,
  updates,
  canManage,
  updateAction,
  isFinePointer,
}: InstalledTabProps) {
  return (
    <section className="min-w-0 space-y-3" aria-labelledby="installed-plugins-heading">
      <InstalledPluginsToolbar actions={actions} updates={updates} canManage={canManage} />
      {canManage && <GlobalAutoUpdateToggle settings={autoUpdate} />}

      <PluginUpdateStatus
        checking={updates.checking}
        lastCheckedAt={updates.lastCheckedAt}
        error={updates.error}
      />

      {canManage && actions.syncErrors.length > 0 && (
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
        canManage={canManage}
        updateAction={updateAction}
        isFinePointer={isFinePointer}
      />
    </section>
  );
}

function InstalledPluginsToolbar({
  actions,
  updates,
  canManage,
}: Pick<InstalledTabProps, "actions" | "updates" | "canManage">) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
      <h3 id="installed-plugins-heading" className={SETTINGS_TYPOGRAPHY.sectionTitle}>
        {t("plugins:installedPlugins")}
      </h3>
      {canManage && (
        <>
          <Button
            data-testid="install-plugin-trigger"
            onClick={actions.openInstall}
            className={settingsActionClassName("ml-auto cursor-pointer md:ml-0")}
          >
            {t("plugins:installPlugin")}
          </Button>
          <div className="flex w-full flex-wrap items-center gap-1 md:ml-auto md:w-auto">
            <Button
              data-testid="plugins-sync-button"
              variant="ghost"
              disabled={actions.syncBusy}
              onClick={actions.handleSync}
              className={settingsActionClassName("cursor-pointer text-muted-foreground")}
            >
              <IconRefresh
                className={`size-4 ${actions.syncBusy ? "animate-spin" : ""}`}
                aria-hidden
              />
              {t("plugins:sync")}
            </Button>
            <Button
              data-testid="plugins-check-updates-button"
              variant="ghost"
              disabled={updates.checking}
              onClick={updates.checkForUpdates}
              className={settingsActionClassName("cursor-pointer text-muted-foreground")}
            >
              <IconRefresh
                className={`size-4 ${updates.checking ? "animate-spin" : ""}`}
                aria-hidden
              />
              {t("plugins:checkForUpdates")}
            </Button>
          </div>
        </>
      )}
    </div>
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
    <SettingsRow
      label={<span className="text-sm">{t("plugins:autoUpdateTitle")}</span>}
      description={t("plugins:autoUpdateDescription")}
      controlId="plugins-auto-update-default"
      touchTarget="switch"
      className="flex-row items-center gap-4 border-y border-border/60"
      controlWrapperClassName="max-md:w-auto"
      control={
        <Switch
          id="plugins-auto-update-default"
          data-testid="plugins-auto-update-default"
          checked={settings.autoUpdateDefault}
          disabled={!settings.loaded}
          onCheckedChange={settings.setDefault}
          className="cursor-pointer"
        />
      }
    />
  );
}

type PluginListProps = {
  list: ReturnType<typeof usePlugins>;
  actions: ReturnType<typeof usePluginActions>;
  autoUpdateDefault: boolean;
  canManage: boolean;
  updates: ReturnType<typeof usePluginUpdates>;
  updateAction: ReturnType<typeof usePluginUpdateAction>;
  isFinePointer: boolean;
};

function PluginList({
  list,
  actions,
  autoUpdateDefault,
  updates,
  canManage,
  updateAction,
  isFinePointer,
}: PluginListProps) {
  const { t } = useTranslation();
  const { items, loaded, loading, error } = list;
  const needsSetup = usePluginSetupStatus(items);

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
    <div className="divide-y divide-border/60">
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
            needsSetup={needsSetup.has(plugin.id)}
            canManage={canManage}
            isFinePointer={isFinePointer}
            uninstallBusy={actions.uninstallBusy}
            onEnable={actions.handleEnable}
            onDisable={actions.handleDisable}
            onConfirmUninstall={async (target) => {
              await actions.confirmUninstall(target);
            }}
            onUpdate={updateAction.runUpdate}
            onSetAutoUpdate={actions.handleSetAutoUpdate}
          />
        );
      })}
    </div>
  );
}
