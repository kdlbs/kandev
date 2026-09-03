"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Input } from "@kandev/ui/input";
import { IconChevronDown, IconSearch } from "@tabler/icons-react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type {
  MCPInheritedSelection,
  MCPServerDefinition,
  MCPSelectionOrigin,
} from "@/lib/types/http-mcp";

export type { MCPInheritedSelection } from "@/lib/types/http-mcp";

type MCPSelectionPickerProps = {
  definitions: MCPServerDefinition[];
  selectedIds: string[];
  onSelectedIdsChange: (ids: string[]) => void;
  inherited?: MCPInheritedSelection[];
  disabled?: boolean;
  label?: string;
  description?: string;
  testId?: string;
};

function originLabel(origin: MCPSelectionOrigin, t: (key: string) => string): string {
  const labels: Record<MCPSelectionOrigin["scope"], string> = {
    profile: t("settings:mcpOriginProfile"),
    repository: t("settings:mcpOriginRepository"),
    task: t("settings:mcpOriginTask"),
    task_session: t("settings:mcpOriginSession"),
  };
  const scope = labels[origin.scope];
  return `${scope}: ${origin.owner_id}`;
}

function ServerRow({
  server,
  checked,
  disabled,
  onChange,
}: {
  server: MCPServerDefinition;
  checked: boolean;
  disabled?: boolean;
  onChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <label className="flex min-h-11 items-center gap-3 rounded-md border p-3 cursor-pointer hover:bg-muted/40">
      <Checkbox
        checked={checked}
        disabled={disabled || !server.enabled}
        onCheckedChange={(value) => onChange(value === true)}
      />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">{server.display_name}</span>
        <span className="block truncate text-xs text-muted-foreground">{server.runtime_name}</span>
      </span>
      {!server.enabled && <Badge variant="outline">{t("settings:mcpDisabled")}</Badge>}
    </label>
  );
}

function PickerBody({
  definitions,
  selectedIds,
  onSelectedIdsChange,
  inherited,
  disabled,
}: Omit<MCPSelectionPickerProps, "label" | "description" | "testId">) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const inheritedIds = useMemo(
    () => new Set((inherited ?? []).map((item) => item.definition.id)),
    [inherited],
  );
  const additions = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return definitions.filter((server) => {
      if (inheritedIds.has(server.id)) return false;
      if (!normalized) return true;
      return `${server.display_name} ${server.runtime_name}`.toLowerCase().includes(normalized);
    });
  }, [definitions, inheritedIds, query]);
  const setChecked = (id: string, checked: boolean) => {
    const next = checked ? [...selectedIds, id] : selectedIds.filter((item) => item !== id);
    onSelectedIdsChange([...new Set(next)]);
  };

  return (
    <div className="min-h-0 space-y-4">
      {(inherited?.length ?? 0) > 0 && (
        <div className="space-y-2">
          <p className="text-xs font-medium text-muted-foreground">{t("settings:mcpInherited")}</p>
          {inherited?.map((item) => (
            <div key={item.definition.id} className="rounded-md border border-dashed p-3">
              <div className="flex items-center gap-2">
                <span className="min-w-0 flex-1 truncate text-sm font-medium">
                  {item.definition.display_name}
                </span>
                <Badge variant="secondary">{t("settings:mcpInheritedBadge")}</Badge>
              </div>
              <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
                {item.origins.map((origin) => (
                  <span key={`${origin.scope}:${origin.owner_id}`}>{originLabel(origin, t)}</span>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
      <div className="space-y-2">
        <p className="text-xs font-medium text-muted-foreground">{t("settings:mcpCurrentScope")}</p>
        <div className="relative">
          <IconSearch className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t("settings:searchMcpServers")}
            aria-label={t("settings:searchMcpServers")}
            className="pl-9"
          />
        </div>
        {additions.length === 0 ? (
          <p className="rounded-md border border-dashed p-4 text-center text-sm text-muted-foreground">
            {t("settings:mcpSelectionEmpty")}
          </p>
        ) : (
          <div className="space-y-2">
            {additions.map((server) => (
              <ServerRow
                key={server.id}
                server={server}
                checked={selectedIds.includes(server.id)}
                disabled={disabled}
                onChange={(checked) => setChecked(server.id, checked)}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

export function MCPSelectionPicker({
  definitions,
  selectedIds,
  onSelectedIdsChange,
  inherited = [],
  disabled,
  label,
  description,
  testId,
}: MCPSelectionPickerProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [open, setOpen] = useState(false);
  const title = label ?? t("settings:mcpServers");
  const pickerBody = (
    <PickerBody
      definitions={definitions}
      selectedIds={selectedIds}
      onSelectedIdsChange={onSelectedIdsChange}
      inherited={inherited}
      disabled={disabled}
    />
  );
  return (
    <div className="min-w-0 space-y-2" data-testid={testId ?? "mcp-selection-picker"}>
      <Button
        type="button"
        variant="outline"
        className="flex min-h-11 w-full cursor-pointer items-center justify-between gap-3 px-3 md:min-h-8"
        onClick={() => setOpen(true)}
        disabled={disabled}
        aria-expanded={open}
      >
        <span className="min-w-0 truncate text-left">{title}</span>
        <span className="flex shrink-0 items-center gap-2">
          <Badge variant="secondary">
            {t("settings:mcpSelectedCount", { count: selectedIds.length })}
          </Badge>
          <IconChevronDown className="size-4" />
        </span>
      </Button>
      {description && <p className="text-xs text-muted-foreground">{description}</p>}
      {!isMobile && open && <div className="rounded-md border p-3">{pickerBody}</div>}
      {isMobile && (
        <Drawer open={open} onOpenChange={setOpen}>
          <DrawerContent className="h-[min(80dvh,42rem)] max-h-[80dvh] overflow-hidden pb-[env(safe-area-inset-bottom)]">
            <DrawerHeader>
              <DrawerTitle>{title}</DrawerTitle>
              <DrawerDescription>
                {description ?? t("settings:mcpSelectionDescription")}
              </DrawerDescription>
            </DrawerHeader>
            <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 pb-4">
              {pickerBody}
            </div>
          </DrawerContent>
        </Drawer>
      )}
    </div>
  );
}
