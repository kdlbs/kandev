"use client";

import { IconArrowsExchange, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Input } from "@kandev/ui/input";
import { unavailableShortcutIcon, type ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import type { ProjectedShortcut } from "@/lib/sidebar/layout-projection";
import type { SidebarLayoutNode } from "@/lib/sidebar/layout-types";

const BUILTIN_LABEL_KEYS: Record<string, string> = {
  home: "sidebar:home",
  new_task: "sidebar:newTask",
  automations: "common:automations",
  canvases: "canvases:canvases",
  integrations: "common:integrations",
};

function nodeTitle(node: SidebarLayoutNode, t: (key: string) => string): string {
  if (node.name) return node.name;
  return node.destinationId && BUILTIN_LABEL_KEYS[node.destinationId]
    ? t(BUILTIN_LABEL_KEYS[node.destinationId])
    : node.id;
}

export function ShortcutSectionMoveMenu({
  shortcut,
  currentNodeId,
  sections,
  onMove,
  t,
}: {
  shortcut: ProjectedShortcut;
  currentNodeId: string;
  sections: SidebarLayoutNode[];
  onMove: (destinationNodeId: string) => void;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  if (sections.length <= 1) return null;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-7 max-md:size-11 [@media(pointer:coarse)]:size-11"
          aria-label={t("settings:moveShortcutToSection", { label: shortcut.label })}
        >
          <IconArrowsExchange className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel>{t("settings:moveToSection")}</DropdownMenuLabel>
        {sections
          .filter((section) => section.id !== currentNodeId)
          .map((section) => (
            <DropdownMenuItem key={section.id} onSelect={() => onMove(section.id)}>
              {nodeTitle(section, t)}
            </DropdownMenuItem>
          ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function SidebarShortcutPicker({
  catalog,
  loading,
  error,
  query,
  onQueryChange,
  onAdd,
  onClose,
  existing,
  t,
}: {
  catalog: ShortcutCatalogEntry[];
  loading: boolean;
  error: string | null;
  query: string;
  onQueryChange: (value: string) => void;
  onAdd: (entry: ShortcutCatalogEntry) => void;
  onClose: () => void;
  existing: string[];
  t: (key: string) => string;
}) {
  const existingSet = new Set(existing);
  const queryLower = query.trim().toLocaleLowerCase();
  const entries = catalog.filter(
    (entry) =>
      !existingSet.has(`${entry.target.kind}:${entry.target.id}`) &&
      (!queryLower || entry.label.toLocaleLowerCase().includes(queryLower)),
  );
  return (
    <div
      className="space-y-2 rounded-md border bg-background p-3"
      role="dialog"
      aria-label={t("settings:addShortcut")}
    >
      <div className="flex items-center gap-2">
        <Input
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          placeholder={t("common:searchEllipsis")}
          aria-label={t("settings:searchShortcuts")}
          className="min-h-7 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        />
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-7 max-md:size-11 [@media(pointer:coarse)]:size-11"
          onClick={onClose}
          aria-label={t("common:close")}
        >
          <IconX className="h-4 w-4" />
        </Button>
      </div>
      {loading && <p className="text-sm text-muted-foreground">{t("common:loading")}</p>}
      {error && (
        <p className="text-sm text-destructive">{t("settings:sidebarShortcutLoadError")}</p>
      )}
      {!loading && !error && entries.length === 0 && (
        <p className="text-sm text-muted-foreground">{t("settings:noMatchingShortcuts")}</p>
      )}
      {entries.length > 0 && (
        <div className="grid gap-1">
          {entries.map((entry) => {
            const Icon = entry.icon ?? unavailableShortcutIcon;
            return (
              <Button
                key={`${entry.target.kind}:${entry.target.id}`}
                type="button"
                variant="ghost"
                className="min-h-7 justify-start gap-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
                onClick={() => onAdd(entry)}
              >
                <Icon className="h-4 w-4 shrink-0" />
                <span className="truncate">{entry.label}</span>
              </Button>
            );
          })}
        </div>
      )}
    </div>
  );
}
