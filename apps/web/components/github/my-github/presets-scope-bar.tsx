"use client";

import { IconBookmark, IconChevronDown, IconDeviceFloppy, IconX } from "@tabler/icons-react";
import type { Icon } from "@tabler/icons-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { PR_PRESETS, ISSUE_PRESETS, type PresetOption } from "./search-bar";
import type { SavedPreset } from "./use-saved-presets";
import type { SidebarSelection } from "./presets-sidebar";

type PresetsScopeBarProps = {
  className?: string;
  selected: SidebarSelection;
  onSelect: (s: SidebarSelection) => void;
  savedPresets: SavedPreset[];
  onDeleteSaved: (id: string) => void;
  canSaveCurrent: boolean;
  onSaveCurrent: () => void;
  prPresets?: PresetOption[];
  issuePresets?: PresetOption[];
};

const PILL_BASE =
  "flex items-center gap-1.5 rounded-md px-2 py-1 text-xs whitespace-nowrap cursor-pointer transition-colors shrink-0";
const PILL_ACTIVE = "bg-muted font-medium text-foreground";
const PILL_IDLE = "text-muted-foreground hover:bg-muted/50 hover:text-foreground";

function Divider() {
  return <div className="mx-0.5 h-5 w-px shrink-0 bg-border" />;
}

function KindSegment({
  kind,
  onChange,
}: {
  kind: "pr" | "issue";
  onChange: (k: "pr" | "issue") => void;
}) {
  return (
    <div className="flex shrink-0 items-center rounded-md border p-0.5 text-xs">
      {(["pr", "issue"] as const).map((value) => (
        <button
          key={value}
          type="button"
          onClick={() => onChange(value)}
          className={cn(
            "rounded px-2 py-0.5 cursor-pointer transition-colors",
            kind === value ? PILL_ACTIVE : "text-muted-foreground hover:text-foreground",
          )}
        >
          {value === "pr" ? "Pull requests" : "Issues"}
        </button>
      ))}
    </div>
  );
}

function PresetPill({
  label,
  Icon,
  active,
  onClick,
}: {
  label: string;
  Icon: Icon;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn(PILL_BASE, active ? PILL_ACTIVE : PILL_IDLE)}
    >
      <Icon className="h-3.5 w-3.5 shrink-0" />
      <span>{label}</span>
    </button>
  );
}

function SavedMenu({
  selected,
  saved,
  onSelect,
  onDeleteSaved,
  canSaveCurrent,
  onSaveCurrent,
  kind,
}: {
  selected: SidebarSelection;
  saved: SavedPreset[];
  onSelect: (s: SidebarSelection) => void;
  onDeleteSaved: (id: string) => void;
  canSaveCurrent: boolean;
  onSaveCurrent: () => void;
  kind: "pr" | "issue";
}) {
  const activeSaved = selected.source === "saved";
  const activeLabel = activeSaved ? saved.find((s) => s.id === selected.id)?.label : null;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          data-testid="github-saved-queries-menu"
          className={cn(PILL_BASE, activeSaved ? PILL_ACTIVE : PILL_IDLE)}
        >
          <IconBookmark className="h-3.5 w-3.5 shrink-0" />
          <span className="max-w-[140px] truncate">{activeLabel ?? "Saved"}</span>
          <IconChevronDown className="h-3 w-3 shrink-0 opacity-60" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        {saved.length === 0 ? (
          <DropdownMenuItem disabled>No saved queries yet</DropdownMenuItem>
        ) : (
          saved.map((s) => (
            <DropdownMenuItem
              key={s.id}
              onClick={() => onSelect({ kind, source: "saved", id: s.id })}
              className="group/saved cursor-pointer gap-2"
            >
              <IconBookmark className="h-3.5 w-3.5 shrink-0" />
              <span className="flex-1 truncate">{s.label}</span>
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  onDeleteSaved(s.id);
                }}
                className="cursor-pointer text-muted-foreground opacity-0 transition-opacity hover:text-foreground group-hover/saved:opacity-100"
                title="Delete saved query"
              >
                <IconX className="h-3.5 w-3.5" />
              </button>
            </DropdownMenuItem>
          ))
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          disabled={!canSaveCurrent}
          onClick={onSaveCurrent}
          className={cn("gap-2", canSaveCurrent && "cursor-pointer")}
        >
          <IconDeviceFloppy className="h-3.5 w-3.5 shrink-0" />
          <span>Save current query</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/**
 * Horizontal scope bar for the /github dashboard — the desktop replacement for
 * the old `w-60` presets rail, which read as a redundant second sidebar next to
 * the global AppSidebar. Bounded, common controls (kind + presets) stay visible
 * as pills; the unbounded saved-queries list collapses into a dropdown so the
 * bar never grows tall. Mobile keeps the vertical PresetsSidebar in a sheet.
 */
export function PresetsScopeBar({
  className,
  selected,
  onSelect,
  savedPresets,
  onDeleteSaved,
  canSaveCurrent,
  onSaveCurrent,
  prPresets = PR_PRESETS,
  issuePresets = ISSUE_PRESETS,
}: PresetsScopeBarProps) {
  const presets = selected.kind === "pr" ? prPresets : issuePresets;
  const saved = savedPresets.filter((p) => p.kind === selected.kind);
  const inbox = presets.filter((p) => p.group === "inbox");
  const created = presets.filter((p) => p.group === "created");

  const onKindChange = (kind: "pr" | "issue") => {
    const fallback = (kind === "pr" ? prPresets : issuePresets)[0]?.value ?? "";
    onSelect({ kind, source: "preset", id: fallback });
  };

  const renderPill = (p: PresetOption) => (
    <PresetPill
      key={`${selected.kind}-${p.value}`}
      label={p.label}
      Icon={p.icon}
      active={selected.source === "preset" && selected.id === p.value}
      onClick={() => onSelect({ kind: selected.kind, source: "preset", id: p.value })}
    />
  );

  return (
    <div
      className={cn("flex items-center gap-1.5 overflow-x-auto px-4 py-2 sm:px-6", className)}
      data-testid="github-presets-scope-bar"
    >
      <KindSegment kind={selected.kind} onChange={onKindChange} />
      <Divider />
      {inbox.map(renderPill)}
      {inbox.length > 0 && created.length > 0 && <Divider />}
      {created.map(renderPill)}
      <div className="ml-auto shrink-0 pl-2">
        <SavedMenu
          selected={selected}
          saved={saved}
          onSelect={onSelect}
          onDeleteSaved={onDeleteSaved}
          canSaveCurrent={canSaveCurrent}
          onSaveCurrent={onSaveCurrent}
          kind={selected.kind}
        />
      </div>
    </div>
  );
}
