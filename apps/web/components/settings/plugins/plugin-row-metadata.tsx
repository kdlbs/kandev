"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Switch } from "@kandev/ui/switch";
import type { MarketplaceEntry, PluginRecord } from "@/lib/types/plugins";
import { SETTINGS_TYPOGRAPHY } from "@/components/settings/settings-typography";
import { PublisherBadge, publisherIdentitiesMatch } from "./plugin-publisher-identity";
import type { PluginRowUpdateState } from "./plugin-row-types";

export function PublisherUpdateNotice({
  current,
  candidate,
}: {
  current?: PluginRecord["publisher_identity"];
  candidate?: MarketplaceEntry["publisher_identity"];
}) {
  const { t } = useTranslation();
  const currentIdentity = current ?? { status: "unverified" as const };
  const candidateIdentity = candidate ?? { status: "unverified" as const };
  const changed = !publisherIdentitiesMatch(currentIdentity, candidateIdentity);
  return (
    <div
      className="space-y-1 rounded-md border border-border/60 bg-muted/30 px-3 py-2 text-xs"
      data-testid="plugin-publisher-update-notice"
    >
      <PublisherVersionLine
        label={t("plugins:installedPublisherLabel")}
        identity={currentIdentity}
      />
      <PublisherVersionLine
        label={t("plugins:candidatePublisherLabel")}
        identity={candidateIdentity}
      />
      {changed && (
        <p className="font-medium text-amber-700 dark:text-amber-400">
          {t("plugins:publisherUpdateChanged")}
        </p>
      )}
    </div>
  );
}

function PublisherVersionLine({
  label,
  identity,
}: {
  label: string;
  identity: NonNullable<PluginRecord["publisher_identity"]>;
}) {
  const { t } = useTranslation();
  const publisher =
    identity.status === "verified" && identity.login
      ? identity.login
      : t("plugins:unverifiedPublisher");
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
      <span>{label}:</span>
      <span className="font-medium text-foreground">{publisher}</span>
      <PublisherBadge identity={identity} />
    </div>
  );
}

export function PluginUpdateInfo({
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

export function PluginAutoUpdateRow({
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
