"use client";

import { useState } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { latestRuntimeVersions, resolveRuntimeActiveVersion } from "@/lib/agent-runtime-update";
import type { AgentUpdateJob, AgentUpdatePreview, AgentUpdateVersion } from "@/lib/api";

const ACTIVE_UPDATE_STATUSES = new Set<AgentUpdateJob["status"]>([
  "queued",
  "resolving",
  "updating",
  "refreshing",
]);

function versionLabel(
  t: TFunction,
  version: AgentUpdateVersion,
  activeVersion?: string,
  defaultVersion?: string,
): string {
  const markers = [
    version.latest ? t("agents:latestRuntimeVersion") : "",
    version.version === activeVersion ? t("agents:activeRuntimeVersionMarker") : "",
    version.version === defaultVersion ? t("agents:kandevDefaultRuntimeVersionMarker") : "",
  ]
    .filter(Boolean)
    .join(", ");
  if (!markers) return version.version;
  return t("agents:runtimeVersionOption", {
    version: version.version,
    markers: t("agents:runtimeVersionMarkerGroup", { markers }),
  });
}

function RuntimeVersionBrowser({
  agentName,
  browserId,
  versions,
  activeVersion,
  defaultVersion,
  selectedVersion,
  selectedUseDefault,
  onSelectTarget,
}: {
  agentName: string;
  browserId: string;
  versions: AgentUpdateVersion[];
  activeVersion?: string;
  defaultVersion?: string;
  selectedVersion?: string;
  selectedUseDefault: boolean;
  onSelectTarget: (targetVersion: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <PopoverContent
      id={browserId}
      align="start"
      className="w-[var(--radix-popover-trigger-width)] max-w-[calc(100vw-2rem)] p-0"
      data-testid={browserId}
      onWheel={(event) => event.stopPropagation()}
    >
      <Command className="h-auto min-h-0 w-full rounded-md bg-transparent" shouldFilter>
        <CommandInput
          autoFocus
          placeholder={t("agents:searchRuntimeVersions")}
          aria-label={t("agents:searchRuntimeVersions")}
        />
        <CommandList className="max-h-[min(18rem,var(--radix-popover-content-available-height))]">
          <CommandEmpty>{t("agents:noMatchingRuntimeVersions")}</CommandEmpty>
          <CommandGroup>
            {versions.map((version) => {
              const isSelected = !selectedUseDefault && selectedVersion === version.version;
              return (
                <CommandItem
                  key={version.version}
                  value={version.version}
                  className="min-h-11 cursor-pointer px-3 py-1.5 sm:min-h-10"
                  aria-selected={isSelected}
                  data-checked={isSelected}
                  data-testid={`agent-update-version-option-${agentName}-${version.version}`}
                  onSelect={() => onSelectTarget(version.version)}
                >
                  <span className="font-mono">{version.version}</span>
                  <span className="text-muted-foreground">
                    {versionLabel(t, version, activeVersion, defaultVersion).slice(
                      version.version.length,
                    )}
                  </span>
                </CommandItem>
              );
            })}
          </CommandGroup>
        </CommandList>
      </Command>
    </PopoverContent>
  );
}

function RuntimeVersionQuickChoices({
  agentName,
  latestVersion,
  activeVersion,
  defaultVersion,
  selectedVersion,
  selectedUseDefault,
  disabled,
  onSelectTarget,
  onSelectDefault,
}: {
  agentName: string;
  latestVersion: string;
  activeVersion?: string;
  defaultVersion?: string;
  selectedVersion?: string;
  selectedUseDefault: boolean;
  disabled: boolean;
  onSelectTarget: (targetVersion: string) => void;
  onSelectDefault: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1">
      <p className="font-medium">{t("agents:runtimeVersionQuickChoices")}</p>
      <div className="grid grid-cols-2 gap-1.5">
        <Button
          type="button"
          variant={selectedVersion === latestVersion ? "secondary" : "outline"}
          className="h-11 min-h-11 min-w-0 cursor-pointer justify-start px-2 text-left text-xs sm:h-10 sm:min-h-10"
          aria-pressed={selectedVersion === latestVersion}
          disabled={disabled}
          onClick={() => onSelectTarget(latestVersion)}
          data-testid={`agent-update-quick-latest-${agentName}`}
        >
          {versionLabel(t, { version: latestVersion, latest: true }, activeVersion, defaultVersion)}
        </Button>
        {defaultVersion && (
          <Button
            type="button"
            variant={selectedUseDefault ? "secondary" : "outline"}
            className="h-11 min-h-11 min-w-0 cursor-pointer justify-start px-2 text-left text-xs sm:h-10 sm:min-h-10"
            aria-pressed={selectedUseDefault}
            disabled={disabled}
            onClick={onSelectDefault}
            data-testid={`agent-update-quick-default-${agentName}`}
          >
            {t("agents:useKandevDefaultVersion", { version: defaultVersion })}
          </Button>
        )}
      </div>
    </div>
  );
}

export function RuntimeVersionPicker({
  agentName,
  preview,
  selectedTarget,
  selectedUseDefault,
  loading,
  starting,
  job,
  onSelectTarget,
  onSelectDefault,
}: {
  agentName: string;
  preview: AgentUpdatePreview;
  selectedTarget: string;
  selectedUseDefault: boolean;
  loading: boolean;
  starting: boolean;
  job?: AgentUpdateJob;
  onSelectTarget: (targetVersion: string) => void;
  onSelectDefault: () => void;
}) {
  const { t } = useTranslation();
  const [browseOpen, setBrowseOpen] = useState(false);
  const versions = latestRuntimeVersions(
    preview.available_versions ?? [{ version: preview.target_version, latest: false }],
  );
  const latestVersion =
    versions.find((version) => version.latest)?.version ?? preview.target_version;
  const activeVersion = resolveRuntimeActiveVersion(preview, job);
  const selectedVersion = selectedUseDefault ? undefined : selectedTarget || preview.target_version;
  const disabled = Boolean(job && ACTIVE_UPDATE_STATUSES.has(job.status)) || starting || loading;
  const browserId = `agent-update-version-browser-${agentName}`;

  const selectTarget = (targetVersion: string) => {
    setBrowseOpen(false);
    onSelectTarget(targetVersion);
  };

  const selectDefault = () => {
    setBrowseOpen(false);
    onSelectDefault();
  };

  return (
    <div className="space-y-2" data-testid={`agent-update-version-picker-${agentName}`}>
      <RuntimeVersionQuickChoices
        agentName={agentName}
        latestVersion={latestVersion}
        activeVersion={activeVersion}
        defaultVersion={preview.default_version}
        selectedVersion={selectedVersion}
        selectedUseDefault={selectedUseDefault}
        disabled={disabled}
        onSelectTarget={selectTarget}
        onSelectDefault={selectDefault}
      />

      <div className="space-y-1">
        <p className="font-medium">{t("agents:selectRuntimeVersion")}</p>
        <Popover modal={false} open={browseOpen} onOpenChange={setBrowseOpen}>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="outline"
              className="h-12 w-full cursor-pointer justify-between px-3"
              aria-expanded={browseOpen}
              aria-controls={browserId}
              disabled={disabled}
              data-testid={`agent-update-version-${agentName}`}
            >
              <span>{t("agents:browseRuntimeVersions")}</span>
              {loading ? (
                <span
                  className="flex items-center"
                  data-testid={`agent-update-version-loading-${agentName}`}
                  role="status"
                >
                  <IconLoader2 className="size-4 animate-spin text-muted-foreground" />
                  <span className="sr-only">{t("agents:checkingLatestRuntimeVersion")}</span>
                </span>
              ) : (
                <IconChevronDown className="size-4 text-muted-foreground" aria-hidden="true" />
              )}
            </Button>
          </PopoverTrigger>
          <RuntimeVersionBrowser
            agentName={agentName}
            browserId={browserId}
            versions={versions}
            activeVersion={activeVersion}
            defaultVersion={preview.default_version}
            selectedVersion={selectedVersion}
            selectedUseDefault={selectedUseDefault}
            onSelectTarget={selectTarget}
          />
        </Popover>
      </div>
    </div>
  );
}
