"use client";

import { IconArrowLeft } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import Link from "@/components/routing/app-link";
import { useRouter } from "@/lib/routing/client-router";
import { usePlugins } from "@/hooks/domains/plugins/use-plugins";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { PluginConfigForm } from "./plugin-config-form";
import { PluginManifestCard } from "./plugin-manifest-card";
import { PluginRepoLink } from "./plugin-repo-link";
import { PluginStatusBadge } from "./plugin-status-badge";
import { UninstallPluginDialog } from "./uninstall-plugin-dialog";
import { usePluginActions } from "./use-plugin-actions";
import { usePluginConfigForm } from "./use-plugin-config-form";
import type { PluginRecord } from "@/lib/types/plugins";

const PLUGINS_SETTINGS_HREF = "/settings/plugins";

/**
 * Per-plugin settings page (Settings > Plugins > <plugin>): manifest
 * overview plus the schema-driven settings form declared by the plugin
 * author via the manifest's config_schema (e.g. a GitHub plugin's PAT).
 * Saving restarts a running plugin so it re-reads config via the Host
 * GetConfig RPC.
 */
export function PluginDetail({ pluginId }: { pluginId: string }) {
  const { items, loaded } = usePlugins();
  const router = useRouter();
  const actions = usePluginActions();
  const plugin = items.find((p) => p.id === pluginId) ?? null;
  const form = usePluginConfigForm(plugin);
  useSettingsSaveContributor({
    id: `plugin-config:${pluginId}`,
    revision: form.revision,
    isDirty: form.isDirty,
    canSave: form.canSave,
    invalidReason: form.invalidReason,
    save: form.handleSave,
    discard: form.discard,
  });

  if (!plugin) {
    return loaded ? <PluginNotFound pluginId={pluginId} /> : null;
  }

  return (
    <div className="space-y-6" data-testid={`plugin-detail-${plugin.id}`}>
      <PluginDetailHeader plugin={plugin} />
      <Separator />

      <PluginSettingsCard plugin={plugin} form={form} busy={actions.busyId === plugin.id} />
      <PluginManifestCard plugin={plugin} />

      <PluginDangerZone plugin={plugin} actions={actions} />
      <UninstallPluginDialog
        target={actions.uninstallTarget}
        busy={actions.uninstallBusy}
        onClose={actions.closeUninstall}
        onConfirm={async () => {
          await actions.confirmUninstall();
          router.push(PLUGINS_SETTINGS_HREF);
        }}
      />
    </div>
  );
}

type PluginDetailHeaderProps = {
  plugin: PluginRecord;
};

function PluginDetailHeader({ plugin }: PluginDetailHeaderProps) {
  return (
    <div className="space-y-3">
      <Link
        href={PLUGINS_SETTINGS_HREF}
        className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground cursor-pointer"
      >
        <IconArrowLeft className="h-4 w-4" />
        Plugins
      </Link>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-2xl font-bold truncate">{plugin.display_name}</h2>
            <PluginStatusBadge status={plugin.status} />
            {plugin.signed === false && (
              <Badge
                variant="outline"
                className="border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400 text-[11px]"
              >
                unsigned
              </Badge>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span className="font-mono">
              {plugin.id} · v{plugin.version}
            </span>
            <PluginRepoLink url={plugin.repo_url} />
          </div>
          {plugin.description && (
            <p className="text-sm text-muted-foreground">{plugin.description}</p>
          )}
        </div>
      </div>
    </div>
  );
}

type PluginSettingsCardProps = {
  plugin: PluginRecord;
  form: ReturnType<typeof usePluginConfigForm>;
  busy: boolean;
};

function PluginSettingsCard({ plugin, form, busy }: PluginSettingsCardProps) {
  return (
    <SettingsCard isDirty={form.isDirty} data-testid="plugin-settings-card">
      <CardHeader>
        <CardTitle className="text-base">Settings</CardTitle>
      </CardHeader>
      <CardContent>
        <PluginSettingsBody plugin={plugin} form={form} busy={busy} />
      </CardContent>
    </SettingsCard>
  );
}

function PluginSettingsBody({ plugin, form, busy }: PluginSettingsCardProps) {
  if (form.fields.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        This plugin does not declare any settings (no <code>config_schema</code> in its manifest).
      </p>
    );
  }
  if (form.configError) {
    return <p className="text-sm text-destructive">{form.configError}</p>;
  }
  if (form.configLoading) {
    return <p className="text-sm text-muted-foreground">Loading settings...</p>;
  }
  return (
    <div className="space-y-4">
      <PluginConfigForm
        fields={form.fields}
        values={form.values}
        initialValues={form.initialValues}
        disabled={busy || form.saveStatus === "loading"}
        onChange={form.handleChange}
      />
      {plugin.status === "active" && (
        <p className="text-xs text-muted-foreground">
          Saving restarts the plugin so the new settings take effect.
        </p>
      )}
    </div>
  );
}

type PluginDangerZoneProps = {
  plugin: PluginRecord;
  actions: ReturnType<typeof usePluginActions>;
};

function PluginDangerZone({ plugin, actions }: PluginDangerZoneProps) {
  const busy = actions.busyId === plugin.id;
  const canEnable = plugin.status === "disabled" || plugin.status === "registered";
  const canDisable = plugin.status === "active" || plugin.status === "error";

  return (
    <div className="flex flex-wrap items-center gap-2">
      {canEnable && (
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer"
          disabled={busy}
          onClick={() => actions.handleEnable(plugin)}
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
          onClick={() => actions.handleDisable(plugin)}
        >
          Disable
        </Button>
      )}
      <Button
        variant="ghost"
        size="sm"
        className="cursor-pointer text-destructive hover:text-destructive"
        disabled={busy}
        onClick={() => actions.openUninstall(plugin)}
      >
        Uninstall
      </Button>
    </div>
  );
}

function PluginNotFound({ pluginId }: { pluginId: string }) {
  return (
    <div className="space-y-4">
      <Link
        href={PLUGINS_SETTINGS_HREF}
        className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground cursor-pointer"
      >
        <IconArrowLeft className="h-4 w-4" />
        Plugins
      </Link>
      <div className="rounded-md border border-dashed p-6 text-sm text-muted-foreground">
        No installed plugin with id <span className="font-mono">{pluginId}</span>.
      </div>
    </div>
  );
}
