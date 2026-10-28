"use client";

import { useTranslation } from "react-i18next";
import { IconRefresh } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { SettingsFieldDescription } from "@/components/settings/settings-typography";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  buildCopilotPluginSelectionRows,
  type CopilotPluginSelectionRow,
} from "@/components/settings/copilot-plugin-selection";
import {
  useCopilotPluginDiscovery,
  type CopilotPluginLaunchSettings,
} from "@/hooks/domains/settings/use-copilot-plugin-discovery";
import { areStringSetsEqual } from "@/lib/string-set-equality";

export function CopilotPluginProfileSelection({
  agentId,
  profileId,
  launchSettings,
  selectedPluginIds,
  savedSelectedPluginIds,
  onSelectedPluginsChange,
}: {
  agentId: string;
  profileId: string;
  launchSettings?: CopilotPluginLaunchSettings;
  selectedPluginIds: string[];
  savedSelectedPluginIds?: string[];
  onSelectedPluginsChange: (ids: string[]) => void;
}) {
  const { t } = useTranslation();
  const { isFinePointer, usesDesktopWorkbench } = useResponsiveBreakpoint();
  const { status, response, stale, refresh } = useCopilotPluginDiscovery({
    agentId,
    profileId,
    launchSettings,
    enabled: Boolean(agentId),
  });
  if (!agentId) return null;

  const rows = buildCopilotPluginSelectionRows(
    {
      plugins: response?.plugins ?? [],
      status: discoveryRowStatus(status, Boolean(response)),
      stale,
    },
    selectedPluginIds,
  );
  const searchable = status === "ready" && !stale;
  const startBlocked = selectedPluginIds.length > 0 && response?.mount_permitted === false;
  const selectionIsDirty =
    savedSelectedPluginIds !== undefined &&
    !areStringSetsEqual(selectedPluginIds, savedSelectedPluginIds);
  const actionHeight = isFinePointer && usesDesktopWorkbench ? "min-h-7" : "min-h-11";
  const rowHeight = isFinePointer && usesDesktopWorkbench ? "min-h-8" : "min-h-11";

  const toggle = (pluginId: string, checked: boolean) => {
    const next = new Set(selectedPluginIds);
    if (checked) next.add(pluginId);
    else next.delete(pluginId);
    onSelectedPluginsChange([...next].sort());
  };

  return (
    <section
      className="space-y-3 rounded-md border p-3"
      data-testid="copilot-plugins-selection"
      data-settings-dirty={selectionIsDirty}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <h3 className="text-sm font-semibold">{t("agents:copilotPluginsTitle")}</h3>
          <SettingsFieldDescription>
            {t("agents:copilotPluginsDescription")}
          </SettingsFieldDescription>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className={`cursor-pointer ${actionHeight}`}
          onClick={() => void refresh()}
          disabled={status === "loading"}
          aria-label={t("agents:refreshCopilotPlugins")}
          data-testid="copilot-plugins-refresh"
        >
          <IconRefresh className={`mr-2 h-4 w-4 ${status === "loading" ? "animate-spin" : ""}`} />
          {t("agents:refreshCopilotPlugins")}
        </Button>
      </div>

      <DiscoveryStatus status={status} stale={stale} isEmpty={rows.length === 0} />
      {startBlocked ? (
        <p
          className="text-xs text-destructive"
          role="status"
          data-testid="copilot-plugins-start-blocked"
        >
          {t("agents:copilotPluginsBlocksStart")}
        </p>
      ) : null}

      {rows.length > 0 ? (
        <div className="space-y-1" role="list">
          {rows.map((row) => (
            <PluginRow
              key={row.id}
              row={row}
              searchable={searchable}
              rowHeight={rowHeight}
              onToggle={toggle}
            />
          ))}
        </div>
      ) : null}

      <p className="text-xs text-muted-foreground">{t("agents:copilotPluginsScopeNote")}</p>
    </section>
  );
}

function discoveryRowStatus(
  status: "loading" | "ready" | "unavailable" | "unsupported",
  hasResponse: boolean,
): "loading" | "ready" | "unavailable" | "unsupported" {
  if (status === "loading") return "loading";
  return hasResponse ? status : "unavailable";
}

function DiscoveryStatus({
  status,
  stale,
  isEmpty,
}: {
  status: "loading" | "ready" | "unavailable" | "unsupported";
  stale: boolean;
  isEmpty: boolean;
}) {
  const { t } = useTranslation();
  if (status === "loading") {
    return (
      <p
        className="text-xs text-muted-foreground"
        role="status"
        data-testid="copilot-plugins-loading"
      >
        {t("agents:copilotPluginsLoading")} {t("agents:copilotPluginsLoadingHint")}
      </p>
    );
  }
  if (status === "unsupported") {
    return (
      <p
        className="text-xs text-muted-foreground"
        role="status"
        data-testid="copilot-plugins-unsupported"
      >
        {t("agents:copilotPluginsUnsupported")}
      </p>
    );
  }
  if (status === "unavailable") {
    return (
      <p className="text-xs text-destructive" role="status" data-testid="copilot-plugins-failed">
        {t("agents:copilotPluginsFailed")}
      </p>
    );
  }
  if (stale) {
    return (
      <p
        className="text-xs text-muted-foreground"
        role="status"
        data-testid="copilot-plugins-stale"
      >
        {t("agents:copilotPluginsStale")}
      </p>
    );
  }
  if (isEmpty) {
    return (
      <p
        className="text-xs text-muted-foreground"
        role="status"
        data-testid="copilot-plugins-empty"
      >
        {t("agents:copilotPluginsEmpty")} {t("agents:copilotPluginsEmptyHint")}
      </p>
    );
  }
  return null;
}

function PluginRow({
  row,
  searchable,
  rowHeight,
  onToggle,
}: {
  row: CopilotPluginSelectionRow;
  searchable: boolean;
  rowHeight: string;
  onToggle: (pluginId: string, checked: boolean) => void;
}) {
  const { t } = useTranslation();
  const canToggle = row.selected || (row.available && searchable);
  const disabled = !canToggle;
  return (
    <label
      className={`flex min-w-0 flex-col gap-1 rounded-md px-2 py-1 sm:flex-row sm:items-start sm:gap-3 ${rowHeight} ${canToggle ? "cursor-pointer hover:bg-muted/40" : "cursor-not-allowed opacity-80"}`}
      data-testid="copilot-plugin-row"
      data-plugin-id={row.id}
      data-unavailable={!row.available}
    >
      <span className="flex min-w-0 flex-1 items-start gap-3">
        <Checkbox
          checked={row.selected}
          disabled={disabled}
          onCheckedChange={(value) => onToggle(row.id, value === true)}
          aria-label={t("agents:copilotPluginsSelect", { name: row.name })}
          className="mt-0.5 flex-none"
        />
        <span className="min-w-0 flex-1 space-y-0.5">
          <span className="block wrap-break-word text-sm">{row.name}</span>
          <PluginMetadata row={row} />
          <PluginReason row={row} />
        </span>
      </span>
      <Badge
        variant={row.available ? "secondary" : "outline"}
        className="w-fit max-w-full whitespace-normal break-words text-left sm:ml-auto sm:shrink-0"
      >
        {row.available
          ? t("agents:copilotPluginsAvailable")
          : t("agents:copilotPluginsUnavailable")}
      </Badge>
    </label>
  );
}

function PluginMetadata({ row }: { row: CopilotPluginSelectionRow }) {
  const { t } = useTranslation();
  const metadata = [row.marketplace, row.version].filter(Boolean).join(" ");
  return (
    <>
      {metadata ? (
        <span className="block wrap-break-word text-xs text-muted-foreground">{metadata}</span>
      ) : null}
      <span className="block text-xs text-muted-foreground">
        {t("agents:copilotPluginsNativeStatus", {
          status: row.nativeEnabled
            ? t("agents:copilotPluginsNativeEnabled")
            : t("agents:copilotPluginsNativeDisabled"),
        })}
      </span>
    </>
  );
}

function PluginReason({ row }: { row: CopilotPluginSelectionRow }) {
  const { t } = useTranslation();
  if (row.availabilityUnchecked) {
    return (
      <span className="block text-xs text-muted-foreground">
        {t("agents:copilotPluginsUnchecked")}. {t("agents:copilotPluginsUncheckedHint")}
      </span>
    );
  }
  if (row.missing) {
    return (
      <span className="block text-xs text-destructive">
        {t("agents:copilotPluginsMissing")}. {t("agents:copilotPluginsMissingHint")}{" "}
        {t("agents:copilotPluginsBlocksStart")}
      </span>
    );
  }
  if (!row.available) {
    if (!row.reasonCode) return null;
    return (
      <span className="block text-xs text-muted-foreground">{t(reasonKey(row.reasonCode))}</span>
    );
  }
  return null;
}

function reasonKey(
  code: string | null,
):
  | "agents:copilotPluginsBuiltinReason"
  | "agents:copilotPluginsExternalReason"
  | "agents:copilotPluginsSourceReason"
  | "agents:copilotPluginsUnsafeReason"
  | "agents:copilotPluginsMetadataReason"
  | "agents:copilotPluginsAmbiguousReason"
  | "agents:copilotPluginsRuntimeReason"
  | "agents:copilotPluginsConflictReason" {
  switch (code) {
    case "builtin_not_mountable":
      return "agents:copilotPluginsBuiltinReason";
    case "external_not_mountable":
      return "agents:copilotPluginsExternalReason";
    case "unsafe_root":
      return "agents:copilotPluginsUnsafeReason";
    case "ambiguous_identity":
      return "agents:copilotPluginsAmbiguousReason";
    case "manual_mount_conflict":
      return "agents:copilotPluginsConflictReason";
    case "metadata_unavailable":
    case "missing_root":
    case "manifest_mismatch":
      return "agents:copilotPluginsMetadataReason";
    case "runtime_unsupported":
    case "policy_unverified":
    case "unsupported_launch_context":
      return "agents:copilotPluginsRuntimeReason";
    default:
      return "agents:copilotPluginsSourceReason";
  }
}
