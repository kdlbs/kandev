"use client";

import { IconX, IconDeviceFloppy, IconBookmark } from "@tabler/icons-react";
import type { Icon } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { PR_PRESETS, ISSUE_PRESETS, type PresetOption, type PresetGroup } from "./search-bar";
import type { SavedPreset } from "./use-saved-presets";
import { useTranslation } from "react-i18next";
import { SavedQueryDefaultButton } from "@/components/integrations/saved-query-default-button";

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
  onToggleSavedDefault: (preset: SavedPreset) => void;
  defaultMutationPending: boolean;
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
  const { t } = useTranslation();
  return (
    <div className="mx-2 mb-3 grid grid-cols-2 rounded-md border p-0.5 text-xs">
      {(["pr", "issue"] as const).map((value) => (
        <button
          key={value}
          type="button"
          onClick={() => onChange(value)}
          className={cn(
            "min-h-11 px-2 py-1 rounded cursor-pointer transition-colors",
            kind === value
              ? "bg-muted font-medium text-foreground"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          {value === "pr" ? t("github:pullRequests") : t("github:issues")}
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
  return (
    <div
      className={cn(
        "group/item mx-1 flex min-w-0 items-center rounded-md text-sm transition-colors",
        active ? "bg-muted font-medium text-foreground" : "text-muted-foreground hover:bg-muted/50",
      )}
    >
      <button
        type="button"
        aria-pressed={active}
        onClick={onClick}
        className="flex min-h-11 min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-md px-2 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <Icon className="h-4 w-4 shrink-0" />
        <span className="min-w-0 flex-1 truncate">{label}</span>
      </button>
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
  const { t } = useTranslation();
  const items = presets.filter((p) => p.group === group);
  if (items.length === 0) return null;
  return (
    <>
      <SectionHeader title={group === "inbox" ? t("github:inbox") : t("github:created")} />
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
  onToggleSavedDefault,
  defaultMutationPending,
}: {
  saved: SavedPreset[];
  selected: SidebarSelection;
  onSelect: (s: SidebarSelection) => void;
  onDelete: (id: string) => void;
  kind: "pr" | "issue";
  canSaveCurrent: boolean;
  onSaveCurrent: () => void;
  onToggleSavedDefault: (preset: SavedPreset) => void;
  defaultMutationPending: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <SectionHeader title={t("github:saved")} />
      {saved.length === 0 && (
        <div className="mx-2 px-2 py-1 text-xs text-muted-foreground/80 italic">
          {t("github:noSavedQueriesYet")}
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
            <div className="flex shrink-0 items-center">
              <SavedQueryDefaultButton
                label={s.label}
                isDefault={s.isDefault}
                disabled={defaultMutationPending}
                size="mobile"
                testId={`github-saved-query-default-${s.id}`}
                onToggle={() => void onToggleSavedDefault(s)}
              />
              <button
                type="button"
                disabled={defaultMutationPending}
                onClick={() => onDelete(s.id)}
                className="flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-wait disabled:opacity-50"
                title={t("integrations:deleteSavedQueryNamed", { label: s.label })}
                aria-label={t("integrations:deleteSavedQueryNamed", { label: s.label })}
              >
                <IconX className="h-4 w-4" />
              </button>
            </div>
          }
        />
      ))}
      <button
        type="button"
        onClick={onSaveCurrent}
        disabled={!canSaveCurrent}
        className={cn(
          "mx-1 mt-1 flex min-h-11 items-center gap-2 rounded-md px-2 py-1.5 text-xs transition-colors",
          canSaveCurrent
            ? "text-muted-foreground hover:bg-muted/50 hover:text-foreground cursor-pointer"
            : "text-muted-foreground/50 cursor-not-allowed",
        )}
        title={canSaveCurrent ? t("github:saveCurrentQuery") : t("github:typeACustomQueryFirst")}
      >
        <IconDeviceFloppy className="h-4 w-4 shrink-0" />
        <span>{t("github:saveCurrentQuery")}</span>
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
  onToggleSavedDefault,
  defaultMutationPending,
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
    <nav className="flex w-full min-w-0 flex-col overflow-x-hidden py-3">
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
        onToggleSavedDefault={onToggleSavedDefault}
        defaultMutationPending={defaultMutationPending}
      />
    </nav>
  );
}
