"use client";

import { useTranslation } from "react-i18next";
import { IconRefresh } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { SettingsFieldDescription } from "@/components/settings/settings-typography";
import { useAgentMcpDiscovery } from "@/hooks/domains/settings/use-agent-mcp-discovery";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  buildCursorMcpSelectionGroups,
  type CursorMcpSelectionGroup,
  type CursorMcpSelectionRow,
} from "@/components/settings/cursor-mcp-selection";

type SelectionMode = "inherit" | "selected";

export function CursorMcpProfileSelection({
  agentId,
  profileId,
  mode,
  selectedServerIds,
  savedMode,
  savedSelectedServerIds,
  onModeChange,
  onSelectedServersChange,
}: {
  agentId: string;
  profileId: string;
  mode: SelectionMode;
  selectedServerIds: string[];
  savedMode?: SelectionMode;
  savedSelectedServerIds?: string[];
  onModeChange: (mode: SelectionMode) => void;
  onSelectedServersChange: (servers: string[]) => void;
}) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const { status, response, refresh } = useAgentMcpDiscovery(agentId, profileId);
  const groups = buildCursorMcpSelectionGroups(
    response ?? {
      agent_id: agentId,
      provider_id: "cursor",
      status: status === "unsupported" ? "unsupported" : "unavailable",
      servers: [],
    },
    selectedServerIds,
  );
  const actionHeight = isFinePointer ? "min-h-7" : "min-h-11";
  const selectionIsDirty =
    savedMode !== undefined &&
    (mode !== (savedMode ?? "inherit") ||
      [...selectedServerIds].sort().join("\u0000") !==
        [...(savedSelectedServerIds ?? [])].sort().join("\u0000"));

  const toggleServer = (serverId: string, checked: boolean) => {
    const next = new Set(selectedServerIds);
    if (checked) next.add(serverId);
    else next.delete(serverId);
    onSelectedServersChange([...next]);
  };

  return (
    <section
      className="space-y-3 rounded-md border p-3"
      data-testid="cursor-mcp-selection"
      data-settings-dirty={selectionIsDirty}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold">{t("agents:cursorMcpSelectionTitle")}</h3>
          <SettingsFieldDescription>
            {t("agents:cursorMcpSelectionDescription")}
          </SettingsFieldDescription>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className={`cursor-pointer ${actionHeight}`}
          onClick={() => void refresh()}
          disabled={status === "loading"}
          data-testid="cursor-mcp-discovery-refresh"
        >
          <IconRefresh className={`mr-2 h-4 w-4 ${status === "loading" ? "animate-spin" : ""}`} />
          {t("agents:refreshMcpDiscovery")}
        </Button>
      </div>

      <ModeSelector
        profileId={profileId}
        mode={mode}
        selectionIsDirty={selectionIsDirty}
        onModeChange={onModeChange}
        isFinePointer={isFinePointer}
      />
      {mode === "inherit" ? (
        <p className="text-xs text-muted-foreground" data-testid="cursor-mcp-inherit-preview">
          {t("agents:cursorMcpInheritancePreview")}
        </p>
      ) : null}
      <DiscoveryStatus status={status} isEmpty={groups.length === 0} />
      <SelectionGroups
        groups={groups}
        mode={mode}
        isFinePointer={isFinePointer}
        onToggle={toggleServer}
      />
    </section>
  );
}

function ModeSelector({
  profileId,
  mode,
  selectionIsDirty,
  onModeChange,
  isFinePointer,
}: {
  profileId: string;
  mode: SelectionMode;
  selectionIsDirty: boolean;
  onModeChange: (mode: SelectionMode) => void;
  isFinePointer: boolean;
}) {
  const { t } = useTranslation();
  return (
    <RadioGroup
      aria-label={t("agents:cursorMcpSelectionMode")}
      value={mode}
      onValueChange={(value) => onModeChange(value as SelectionMode)}
      className="grid gap-2 sm:grid-cols-2"
      data-settings-dirty={selectionIsDirty}
    >
      <ModeOption
        id={`${profileId}-mcp-mode-inherit`}
        value="inherit"
        selected={mode === "inherit"}
        label={t("agents:cursorMcpInheritEnabled")}
        description={t("agents:cursorMcpInheritEnabledDescription")}
        isFinePointer={isFinePointer}
      />
      <ModeOption
        id={`${profileId}-mcp-mode-selected`}
        value="selected"
        selected={mode === "selected"}
        label={t("agents:cursorMcpSelectedOnly")}
        description={t("agents:cursorMcpSelectedOnlyDescription")}
        isFinePointer={isFinePointer}
      />
    </RadioGroup>
  );
}

function DiscoveryStatus({
  status,
  isEmpty,
}: {
  status: "loading" | "ready" | "unavailable" | "unsupported";
  isEmpty: boolean;
}) {
  const { t } = useTranslation();
  if (status === "loading") {
    return (
      <p
        className="text-xs text-muted-foreground"
        role="status"
        data-testid="cursor-mcp-discovery-loading"
      >
        {t("agents:loadingMcpDiscovery")}
      </p>
    );
  }
  if (status === "unavailable") {
    return (
      <p
        className="text-xs text-destructive"
        role="status"
        data-testid="cursor-mcp-discovery-unavailable"
      >
        {t("agents:mcpDiscoveryUnavailable")}
      </p>
    );
  }
  if (status === "unsupported") {
    return (
      <p
        className="text-xs text-muted-foreground"
        role="status"
        data-testid="cursor-mcp-discovery-unsupported"
      >
        {t("agents:mcpDiscoveryUnsupported")}
      </p>
    );
  }
  if (isEmpty) {
    return (
      <p className="text-xs text-muted-foreground" data-testid="cursor-mcp-discovery-empty">
        {t("agents:noMcpServersDiscovered")}
      </p>
    );
  }
  return null;
}

function SelectionGroups({
  groups,
  mode,
  isFinePointer,
  onToggle,
}: {
  groups: CursorMcpSelectionGroup[];
  mode: SelectionMode;
  isFinePointer: boolean;
  onToggle: (serverId: string, checked: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      {groups.map((group) => (
        <fieldset key={group.id} className="min-w-0 space-y-1">
          <legend className="mb-1 text-xs font-semibold text-muted-foreground">
            {group.pluginName ?? t(groupTitleKey(group.id))}
          </legend>
          {group.servers.map((server) => (
            <SelectionServerRow
              key={server.id}
              server={server}
              mode={mode}
              isFinePointer={isFinePointer}
              onToggle={onToggle}
            />
          ))}
        </fieldset>
      ))}
    </div>
  );
}

function groupTitleKey(
  groupId: string,
): "agents:unavailableMcpSelections" | "agents:otherMcpServers" {
  return groupId === "unavailable-selections"
    ? "agents:unavailableMcpSelections"
    : "agents:otherMcpServers";
}

function SelectionServerRow({
  server,
  mode,
  isFinePointer,
  onToggle,
}: {
  server: CursorMcpSelectionRow;
  mode: SelectionMode;
  isFinePointer: boolean;
  onToggle: (serverId: string, checked: boolean) => void;
}) {
  const { t } = useTranslation();
  const checked = mode === "inherit" && !server.unavailable ? true : server.selected;
  const badgeVariant = server.credentialsAvailable && !server.unavailable ? "secondary" : "outline";
  return (
    <label
      className={`flex ${isFinePointer ? "min-h-8 flex-row items-center gap-3" : "min-h-11 flex-col items-stretch gap-2"} min-w-0 rounded-md px-2 py-1 hover:bg-muted/40 ${mode === "selected" ? "cursor-pointer" : "cursor-default"}`}
      data-testid="cursor-mcp-server-row"
      data-server-id={server.id}
      data-unavailable={server.unavailable}
    >
      <span className="flex min-w-0 items-start gap-3">
        <Checkbox
          checked={checked}
          disabled={mode !== "selected"}
          onCheckedChange={(value) => onToggle(server.id, value === true)}
          aria-label={t("agents:selectMcpServer", { name: server.name })}
          className="mt-0.5 flex-none"
        />
        <span className="min-w-0 flex-1">
          <span className="block break-words text-sm">{server.name}</span>
          {server.unavailable ? (
            <span className="block text-xs text-muted-foreground">
              {t("agents:savedMcpServerUnavailable")}
            </span>
          ) : null}
        </span>
      </span>
      <Badge
        variant={badgeVariant}
        className={`max-w-full whitespace-normal break-words text-left ${isFinePointer ? "ml-auto shrink-0" : "ml-7 self-start"}`}
      >
        {serverBadgeLabel(server, t)}
      </Badge>
    </label>
  );
}

function serverBadgeLabel(
  server: CursorMcpSelectionRow,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (server.unavailable) return t("agents:unavailable");
  if (server.credentialsAvailable) return t("agents:mcpCredentialsAvailable");
  return t("agents:noReusableMcpCredentials");
}

function ModeOption({
  id,
  value,
  selected,
  label,
  description,
  isFinePointer,
}: {
  id: string;
  value: SelectionMode;
  selected: boolean;
  label: string;
  description: string;
  isFinePointer: boolean;
}) {
  return (
    <Label
      htmlFor={id}
      className={`flex ${isFinePointer ? "min-h-8" : "min-h-11"} cursor-pointer items-start gap-3 rounded-md border p-3 transition-colors ${selected ? "border-primary bg-primary/5" : "border-border hover:bg-muted/30"}`}
    >
      <RadioGroupItem id={id} value={value} className="mt-0.5 flex-none" />
      <span className="min-w-0 space-y-1">
        <span className="block text-sm font-medium">{label}</span>
        <span className="block text-xs text-muted-foreground">{description}</span>
      </span>
    </Label>
  );
}
