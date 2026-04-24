"use client";

import { IconX, IconDeviceFloppy, IconBookmark } from "@tabler/icons-react";
import type { Icon } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { PR_PRESETS, ISSUE_PRESETS, type PresetOption, type PresetGroup } from "./search-bar";
import type { SavedPreset } from "./use-saved-presets";

export type SidebarSelection = {
  kind: "pr" | "issue";
  source: "preset" | "saved";
  id: string;
};

type PresetsSidebarProps = {
  selected: SidebarSelection;
  onSelect: (s: SidebarSelection) => void;
  savedPresets: SavedPreset[];
  onDeleteSaved: (id: string) => void;
  canSaveCurrent: boolean;
  onSaveCurrent: () => void;
  prPresets?: PresetOption[];
  issuePresets?: PresetOption[];
};

function KindToggle({
  kind,
  onChange,
}: {
  kind: "pr" | "issue";
  onChange: (k: "pr" | "issue") => void;
}) {
  return (
    <div className="mx-2 mb-3 grid grid-cols-2 rounded-md border p-0.5 text-xs">
      {(["pr", "issue"] as const).map((value) => (
        <button
          key={value}
          type="button"
          onClick={() => onChange(value)}
          className={cn(
            "px-2 py-1 rounded cursor-pointer transition-colors",
            kind === value
              ? "bg-muted font-medium text-foreground"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          {value === "pr" ? "Pull requests" : "Issues"}
        </button>
      ))}
    </div>
  );
}

function SectionHeader({ title }: { title: string }) {
  return (
    <div className="px-2 mt-3 mb-1 text-[11px] uppercase tracking-wider text-muted-foreground font-medium">
      {title}
    </div>
  );
}

function PresetItem({
  label,
  Icon,
  active,
  onClick,
  trailing,
}: {
  label: string;
  Icon: Icon;
  active: boolean;
  onClick: () => void;
  trailing?: React.ReactNode;
}) {
  const onKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onClick();
    }
  };
  return (
    <div
      role="button"
      tabIndex={0}
      aria-pressed={active}
      className={cn(
        "group/item mx-1 flex items-center gap-2 rounded-md px-2 py-1.5 text-sm cursor-pointer transition-colors",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        active ? "bg-muted font-medium text-foreground" : "text-muted-foreground hover:bg-muted/50",
      )}
      onClick={onClick}
      onKeyDown={onKeyDown}
    >
      <Icon className="h-4 w-4 shrink-0" />
      <span className="truncate flex-1">{label}</span>
      {trailing}
    </div>
  );
}

function PresetGroupList({
  presets,
  group,
  selected,
  onSelect,
  kind,
}: {
  presets: PresetOption[];
  group: PresetGroup;
  selected: SidebarSelection;
  onSelect: (s: SidebarSelection) => void;
  kind: "pr" | "issue";
}) {
  const items = presets.filter((p) => p.group === group);
  if (items.length === 0) return null;
  return (
    <>
      <SectionHeader title={group === "inbox" ? "Inbox" : "Created"} />
      {items.map((p) => (
        <PresetItem
          key={`${kind}-${p.value}`}
          label={p.label}
          Icon={p.icon}
          active={selected.source === "preset" && selected.id === p.value}
          onClick={() => onSelect({ kind, source: "preset", id: p.value })}
        />
      ))}
    </>
  );
}

function SavedSection({
  saved,
  selected,
  onSelect,
  onDelete,
  kind,
  canSaveCurrent,
  onSaveCurrent,
}: {
  saved: SavedPreset[];
  selected: SidebarSelection;
  onSelect: (s: SidebarSelection) => void;
  onDelete: (id: string) => void;
  kind: "pr" | "issue";
  canSaveCurrent: boolean;
  onSaveCurrent: () => void;
}) {
  return (
    <>
      <SectionHeader title="Saved" />
      {saved.length === 0 && (
        <div className="mx-2 px-2 py-1 text-xs text-muted-foreground/80 italic">
          No saved queries yet.
        </div>
      )}
      {saved.map((s) => (
        <PresetItem
          key={s.id}
          label={s.label}
          Icon={IconBookmark}
          active={selected.source === "saved" && selected.id === s.id}
          onClick={() => onSelect({ kind, source: "saved", id: s.id })}
          trailing={
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onDelete(s.id);
              }}
              className="opacity-0 group-hover/item:opacity-100 transition-opacity text-muted-foreground hover:text-foreground cursor-pointer"
              title="Delete saved query"
            >
              <IconX className="h-3.5 w-3.5" />
            </button>
          }
        />
      ))}
      <button
        type="button"
        onClick={onSaveCurrent}
        disabled={!canSaveCurrent}
        className={cn(
          "mx-1 mt-1 flex items-center gap-2 rounded-md px-2 py-1.5 text-xs transition-colors",
          canSaveCurrent
            ? "text-muted-foreground hover:bg-muted/50 hover:text-foreground cursor-pointer"
            : "text-muted-foreground/50 cursor-not-allowed",
        )}
        title={canSaveCurrent ? "Save current query" : "Type a custom query first"}
      >
        <IconDeviceFloppy className="h-4 w-4 shrink-0" />
        <span>Save current query</span>
      </button>
    </>
  );
}

export function PresetsSidebar({
  selected,
  onSelect,
  savedPresets,
  onDeleteSaved,
  canSaveCurrent,
  onSaveCurrent,
  prPresets = PR_PRESETS,
  issuePresets = ISSUE_PRESETS,
}: PresetsSidebarProps) {
  const presets = selected.kind === "pr" ? prPresets : issuePresets;
  const saved = savedPresets.filter((p) => p.kind === selected.kind);
  const onKindChange = (kind: "pr" | "issue") => {
    const fallback = (kind === "pr" ? prPresets : issuePresets)[0]?.value ?? "";
    onSelect({ kind, source: "preset", id: fallback });
  };
  return (
    <nav className="flex flex-col py-3">
      <KindToggle kind={selected.kind} onChange={onKindChange} />
      <PresetGroupList
        presets={presets}
        group="inbox"
        selected={selected}
        onSelect={onSelect}
        kind={selected.kind}
      />
      <PresetGroupList
        presets={presets}
        group="created"
        selected={selected}
        onSelect={onSelect}
        kind={selected.kind}
      />
      <SavedSection
        saved={saved}
        selected={selected}
        onSelect={onSelect}
        onDelete={onDeleteSaved}
        kind={selected.kind}
        canSaveCurrent={canSaveCurrent}
        onSaveCurrent={onSaveCurrent}
      />
    </nav>
  );
}
