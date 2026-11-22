"use client";

import { useState } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { IconChevronRight, IconLoader2 } from "@tabler/icons-react";
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
import { ACTIVE_UPDATE_STATUSES, resolveRuntimeActiveVersion } from "@/lib/agent-runtime-update";
import type { AgentUpdateJob, AgentUpdatePreview, AgentUpdateVersion } from "@/lib/api";
import { settingsActionClassName } from "./settings-control";
import { DEFAULT_RUNTIME_TARGET } from "./use-agent-update-dialog-state";

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
  onSelectDefault,
}: {
  agentName: string;
  browserId: string;
  versions: AgentUpdateVersion[];
  activeVersion?: string;
  defaultVersion?: string;
  selectedVersion?: string;
  selectedUseDefault: boolean;
  onSelectTarget: (targetVersion: string) => void;
  onSelectDefault: () => void;
}) {
  const { t } = useTranslation();
  return (
    <PopoverContent
      id={browserId}
      align="end"
      className="w-72 max-w-[calc(100dvw-2rem)] p-0"
      data-testid={browserId}
      onWheel={(event) => event.stopPropagation()}
    >
      <Command
        className="h-auto min-h-0 w-full rounded-md bg-transparent"
        defaultValue={selectedUseDefault ? DEFAULT_RUNTIME_TARGET : selectedVersion}
        shouldFilter
      >
        <CommandInput
          autoFocus
          placeholder={t("agents:searchRuntimeVersions")}
          aria-label={t("agents:searchRuntimeVersions")}
        />
        <CommandList className="max-h-[min(18rem,var(--radix-popover-content-available-height))]">
          <CommandEmpty>{t("agents:noMatchingRuntimeVersions")}</CommandEmpty>
          <CommandGroup>
            {defaultVersion && (
              <CommandItem
                value={DEFAULT_RUNTIME_TARGET}
                keywords={[defaultVersion, t("agents:useKandevDefault")]}
                className="min-h-10 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11 cursor-pointer px-3 py-1.5"
                aria-selected={selectedUseDefault}
                data-checked={selectedUseDefault}
                data-testid={`agent-update-version-default-${agentName}`}
                onSelect={onSelectDefault}
              >
                {t("agents:useKandevDefaultVersion", { version: defaultVersion })}
              </CommandItem>
            )}
            {versions.map((version) => {
              const isSelected = !selectedUseDefault && selectedVersion === version.version;
              return (
                <CommandItem
                  key={version.version}
                  value={version.version}
                  className="min-h-10 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11 cursor-pointer px-3 py-1.5"
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
  let versions = preview.available_versions ?? [];
  if (versions.length === 0 && preview.target_version) {
    versions = [{ version: preview.target_version, latest: false }];
  }
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
    <div className="min-w-0" data-testid={`agent-update-version-picker-${agentName}`}>
      <Popover modal={false} open={browseOpen} onOpenChange={setBrowseOpen}>
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="outline"
            className={settingsActionClassName(
              "max-w-full max-md:min-h-11 [@media(pointer:coarse)]:min-h-11",
            )}
            aria-expanded={browseOpen}
            aria-controls={browserId}
            disabled={disabled}
            data-testid={`agent-update-version-${agentName}`}
          >
            {t("agents:changeRuntimeVersion")}
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
              <IconChevronRight className="size-3.5 text-muted-foreground" aria-hidden="true" />
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
          onSelectDefault={selectDefault}
        />
      </Popover>
    </div>
  );
}
