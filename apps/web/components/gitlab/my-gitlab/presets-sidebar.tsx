"use client";

import { IconX, IconDeviceFloppy, IconBookmark } from "@tabler/icons-react";
import type { Icon } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { MR_PRESETS, ISSUE_PRESETS, type PresetOption, type PresetGroup } from "./presets";
import type { SavedPreset } from "./use-saved-presets";
import { useTranslation } from "react-i18next";

export type SidebarSelection = {
  kind: "mr" | "issue";
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
  mrPresets?: PresetOption[];
  issuePresets?: PresetOption[];
};

function KindToggle({
  kind,
  onChange,
}: {
  kind: "mr" | "issue";
  onChange: (k: "mr" | "issue") => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="mx-2 mb-3 grid grid-cols-2 rounded-md border p-0.5 text-xs"
      data-testid="gitlab-kind-toggle"
    >
      {(["mr", "issue"] as const).map((value) => (
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
          data-testid={`gitlab-kind-${value}`}
        >
          {value === "mr" ? t("gitlab:mergeRequests") : t("gitlab:issues")}
        </button>
      ))}
    </div>
  );
}

/**
 * `id` is a stable section identifier and `title` is the visible label. They
 * were one value: the testid was built as `gitlab-section-${title.toLowerCase()}`,
 * so translating the title moved the selector with it — under pseudo the inbox
 * section became `gitlab-section-ĩńƀōx`. apps/web/AGENTS.md is explicit that a
 * `data-testid` is never translated; deriving one from copy translates it by
 * the back door.
 */
function SectionHeader({ id, title }: { id: string; title: string }) {
  return (
    <div
      className="px-2 mt-3 mb-1 text-[11px] uppercase tracking-wider text-muted-foreground font-medium"
      data-testid={`gitlab-section-${id}`}
    >
      {title}
    </div>
  );
}

function PresetItem({
  label,
  Icon: ItemIcon,
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
    // Ignore keys originating from descendants (e.g. the nested delete button),
    // otherwise Enter/Space on delete would also fire row selection.
    if (e.currentTarget !== e.target) return;
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
      <ItemIcon className="h-4 w-4 shrink-0" />
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
  kind: "mr" | "issue";
}) {
  const { t } = useTranslation();
  const items = presets.filter((p) => p.group === group);
  if (items.length === 0) return null;
  return (
    <>
      <SectionHeader
        id={group}
        title={group === "inbox" ? t("gitlab:inbox") : t("gitlab:created")}
      />
      {items.map((p) => (
        <PresetItem
          key={`${kind}-${p.value}`}
          label={t(p.labelKey)}
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
  kind: "mr" | "issue";
  canSaveCurrent: boolean;
  onSaveCurrent: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <SectionHeader id="saved" title={t("gitlab:saved")} />
      {saved.length === 0 && (
        <div className="mx-2 px-2 py-1 text-xs text-muted-foreground/80 italic">
          {t("gitlab:noSavedQueriesYet")}
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
              title={t("gitlab:deleteSavedQuery")}
              aria-label={t("gitlab:deleteSavedQuery2", { label: s.label })}
              data-testid={`gitlab-saved-delete-${s.id}`}
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
        title={canSaveCurrent ? t("gitlab:saveCurrentQuery") : t("gitlab:typeACustomQueryFirst")}
        data-testid="gitlab-save-current-query"
      >
        <IconDeviceFloppy className="h-4 w-4 shrink-0" />
        <span>{t("gitlab:saveCurrentQuery")}</span>
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
  mrPresets = MR_PRESETS,
  issuePresets = ISSUE_PRESETS,
}: PresetsSidebarProps) {
  const presets = selected.kind === "mr" ? mrPresets : issuePresets;
  const saved = savedPresets.filter((p) => p.kind === selected.kind);
  const onKindChange = (kind: "mr" | "issue") => {
    const fallback = (kind === "mr" ? mrPresets : issuePresets)[0]?.value ?? "";
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
