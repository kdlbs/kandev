"use client";

import { useCallback, useEffect, useRef, useState, type RefObject, type ReactNode } from "react";
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
import { IntegrationIcon, type IntegrationIconName } from "./integration-icon";
import { useTranslation } from "react-i18next";
import { SavedQueryDefaultDropdownItem } from "./saved-query-default-button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { isActionConfirmationTarget } from "@/components/confirmation/action-confirm-popover";
import { SavedTaskViewDeleteConfirmation } from "@/components/confirmation/saved-task-view-delete-confirmation";
import {
  useSavedTaskViewDeleteConfirmation,
  type SavedTaskViewDeleteTarget,
} from "@/components/confirmation/use-saved-task-view-delete-confirmation";

/**
 * Shared, domain-agnostic scope bar for the integration dashboards (/github,
 * /gitlab). It's the desktop replacement for the old `w-60` presets rail, which
 * read as a redundant second sidebar next to the global AppSidebar. Bounded,
 * common controls (kind + presets) stay visible as pills; the unbounded
 * saved-queries list collapses into a dropdown so the bar never grows tall.
 *
 * Each integration wraps this with its own kinds/presets/types — see
 * `my-github/presets-scope-bar.tsx` and `my-gitlab/presets-scope-bar.tsx`.
 */
export type ScopePreset = {
  value: string;
  label: string;
  icon?: Icon;
  iconName?: IntegrationIconName;
  group: "inbox" | "created";
};

export type ScopeSelection<K extends string> = {
  kind: K;
  source: "preset" | "saved";
  id: string;
};

export type ScopeSavedPreset<K extends string> = {
  id: string;
  kind: K;
  label: string;
  isDefault?: boolean;
};

const PILL_BASE =
  "flex items-center gap-1.5 rounded-md px-2 py-1 text-xs whitespace-nowrap cursor-pointer transition-colors shrink-0";
const PILL_ACTIVE = "bg-muted font-medium text-foreground";
const PILL_IDLE = "text-muted-foreground hover:bg-muted/50 hover:text-foreground";

function Divider() {
  return <div className="mx-0.5 h-5 w-px shrink-0 bg-border" />;
}

function KindSegment<K extends string>({
  kinds,
  active,
  onChange,
}: {
  kinds: ReadonlyArray<{ value: K; label: string }>;
  active: K;
  onChange: (k: K) => void;
}) {
  return (
    <div className="flex shrink-0 items-center rounded-md border p-0.5 text-xs">
      {kinds.map(({ value, label }) => (
        <button
          key={value}
          type="button"
          onClick={() => onChange(value)}
          className={cn(
            "rounded px-2 py-0.5 cursor-pointer transition-colors",
            active === value ? PILL_ACTIVE : "text-muted-foreground hover:text-foreground",
          )}
        >
          {label}
        </button>
      ))}
    </div>
  );
}

function PresetPill({
  label,
  Icon = IconBookmark,
  iconName,
  active,
  onClick,
}: {
  label: string;
  Icon?: Icon;
  iconName?: IntegrationIconName;
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
      {iconName ? (
        <IntegrationIcon name={iconName} className="h-3.5 w-3.5 shrink-0" />
      ) : (
        <Icon className="h-3.5 w-3.5 shrink-0" />
      )}
      <span>{label}</span>
    </button>
  );
}

function SavedMenuTrigger({
  testId,
  active,
  label,
}: {
  testId: string;
  active: boolean;
  label: string | null;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenuTrigger asChild>
      <button
        type="button"
        data-testid={testId}
        className={cn(PILL_BASE, active ? PILL_ACTIVE : PILL_IDLE)}
      >
        <IconBookmark className="h-3.5 w-3.5 shrink-0" />
        <span className="max-w-[140px] truncate">{label ?? t("integrations:savedQueries")}</span>
        <IconChevronDown className="h-3 w-3 shrink-0 opacity-60" />
      </button>
    </DropdownMenuTrigger>
  );
}

function useSavedMenuSave(onSaveCurrent: () => void) {
  const pending = useRef(false);
  const frame = useRef(0);
  useEffect(
    () => () => {
      pending.current = false;
      cancelAnimationFrame(frame.current);
    },
    [],
  );
  return {
    request: () => {
      pending.current = true;
    },
    onCloseAutoFocus: () => {
      if (!pending.current) return;
      pending.current = false;
      frame.current = requestAnimationFrame(onSaveCurrent);
    },
  };
}

function SavedMenu<K extends string>({
  testId,
  selected,
  saved,
  onSelect,
  onDeleteSaved,
  canSaveCurrent,
  onSaveCurrent,
  onToggleSavedDefault,
  defaultMutationPendingId,
  savedStatus,
}: {
  testId: string;
  selected: ScopeSelection<K>;
  saved: ScopeSavedPreset<K>[];
  onSelect: (s: ScopeSelection<K>) => void;
  onDeleteSaved: (id: string) => void;
  canSaveCurrent: boolean;
  onSaveCurrent: () => void;
  onToggleSavedDefault?: (id: string) => void;
  defaultMutationPendingId: string | null;
  savedStatus?: ReactNode;
}) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const [menuOpen, setMenuOpen] = useState(false);
  const save = useSavedMenuSave(onSaveCurrent);
  const deletion = useSavedTaskViewDeleteConfirmation(saved);
  const menuContentRef = useRef<HTMLDivElement>(null);
  const defaultMutationPending = defaultMutationPendingId !== null;
  const activeSaved = selected.source === "saved";
  const activeLabel = activeSaved ? (saved.find((s) => s.id === selected.id)?.label ?? null) : null;
  return (
    <DropdownMenu
      open={menuOpen}
      onOpenChange={(open) => {
        setMenuOpen(open);
        if (!open) deletion.close();
      }}
    >
      <SavedMenuTrigger testId={testId} active={activeSaved} label={activeLabel} />
      <DropdownMenuContent
        ref={menuContentRef}
        align="end"
        className="w-56"
        onCloseAutoFocus={save.onCloseAutoFocus}
        onFocusOutside={(event) => {
          if (isActionConfirmationTarget(event.target)) event.preventDefault();
        }}
        onInteractOutside={(event) => {
          if (isActionConfirmationTarget(event.target)) event.preventDefault();
        }}
      >
        {savedStatus}
        {saved.length === 0
          ? !savedStatus && (
              <DropdownMenuItem disabled>{t("integrations:noSavedQueriesYet")}</DropdownMenuItem>
            )
          : saved.map((preset) => (
              <SavedMenuEntry
                key={preset.id}
                preset={preset}
                isFinePointer={isFinePointer}
                deletion={deletion}
                defaultMutationPending={defaultMutationPending}
                defaultMutationPendingForPreset={defaultMutationPendingId === preset.id}
                onSelect={() => onSelect({ kind: preset.kind, source: "saved", id: preset.id })}
                onToggleDefault={
                  onToggleSavedDefault ? () => onToggleSavedDefault(preset.id) : undefined
                }
                onDeleteSaved={onDeleteSaved}
              />
            ))}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          disabled={!canSaveCurrent}
          onSelect={save.request}
          className={cn("gap-2", canSaveCurrent && "cursor-pointer")}
        >
          <IconDeviceFloppy className="h-3.5 w-3.5 shrink-0" />
          <span>{t("integrations:saveCurrentQuery")}</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
      {isFinePointer && deletion.target ? (
        <SavedTaskViewDeleteConfirmation
          target={deletion.target}
          presentation="popover"
          open
          anchorRef={deletion.anchorRef}
          focusBoundaryRef={menuContentRef}
          confirmDisabled={defaultMutationPending}
          onOpenChange={(open) => {
            if (!open) deletion.close();
          }}
          onConfirm={onDeleteSaved}
        />
      ) : null}
    </DropdownMenu>
  );
}

type SavedMenuDeletion = {
  target: SavedTaskViewDeleteTarget | null;
  anchorRef: RefObject<HTMLElement | null>;
  close: () => void;
  registerAnchor: (id: string, element: HTMLElement | null) => void;
  request: (target: SavedTaskViewDeleteTarget) => void;
};

function SavedMenuEntry({
  preset,
  isFinePointer,
  deletion,
  defaultMutationPending,
  defaultMutationPendingForPreset,
  onSelect,
  onToggleDefault,
  onDeleteSaved,
}: {
  preset: ScopeSavedPreset<string>;
  isFinePointer: boolean;
  deletion: SavedMenuDeletion;
  defaultMutationPending: boolean;
  defaultMutationPendingForPreset: boolean;
  onSelect: () => void;
  onToggleDefault?: () => void;
  onDeleteSaved: (id: string) => void;
}) {
  const { t } = useTranslation();
  if (!isFinePointer && deletion.target?.id === preset.id) {
    return (
      <div role="none" className="min-w-0 p-1">
        <SavedTaskViewDeleteConfirmation
          target={deletion.target}
          presentation="inline"
          open
          anchorRef={deletion.anchorRef}
          confirmDisabled={defaultMutationPending}
          onOpenChange={(open) => {
            if (!open) deletion.close();
          }}
          onConfirm={onDeleteSaved}
        />
      </div>
    );
  }
  const deleteLabel = t("integrations:deleteSavedQueryNamed", { label: preset.label });
  const accessibleDeleteLabel = defaultMutationPending
    ? t("integrations:savedQueryDefaultUpdateInProgress", { action: deleteLabel })
    : deleteLabel;
  return (
    <div role="none" className="group/saved flex items-center gap-0.5">
      <DropdownMenuItem onSelect={onSelect} className="min-w-0 flex-1 cursor-pointer gap-2">
        <IconBookmark className="h-3.5 w-3.5 shrink-0" />
        <span className="flex-1 truncate">{preset.label}</span>
      </DropdownMenuItem>
      {onToggleDefault && (
        <SavedQueryDefaultDropdownItem
          label={preset.label}
          isDefault={preset.isDefault === true}
          disabled={defaultMutationPending}
          pending={defaultMutationPendingForPreset}
          testId={`saved-query-default-${preset.id}`}
          onToggle={onToggleDefault}
        />
      )}
      {/* A peer Radix item keeps delete keyboard-reachable without selecting the preset. */}
      <DropdownMenuItem
        ref={(element) => deletion.registerAnchor(preset.id, element)}
        disabled={defaultMutationPending}
        onSelect={(event) => {
          event.preventDefault();
          deletion.request({ id: preset.id, label: preset.label });
        }}
        className={cn(
          "shrink-0 cursor-pointer justify-center p-0 text-muted-foreground transition-opacity hover:text-foreground data-[disabled]:cursor-wait data-[disabled]:opacity-50",
          isFinePointer
            ? "h-7 min-h-7 w-7 opacity-0 focus:opacity-100 group-hover/saved:opacity-100 group-hover/saved:data-[disabled]:opacity-50"
            : "min-h-12 min-w-12 opacity-100",
        )}
        title={accessibleDeleteLabel}
        aria-label={accessibleDeleteLabel}
      >
        <IconX className="h-3.5 w-3.5" />
      </DropdownMenuItem>
    </div>
  );
}

type SavedDefaultActionProps =
  | {
      /** Emits only the stable id; domain wrappers own richer saved-query data. */
      onToggleSavedDefault: (id: string) => void;
      defaultMutationPendingId: string | null;
    }
  | {
      onToggleSavedDefault?: never;
      defaultMutationPendingId?: never;
    };

export type IntegrationScopeBarProps<K extends string> = {
  className?: string;
  testId: string;
  savedMenuTestId: string;
  kinds: ReadonlyArray<{ value: K; label: string }>;
  selected: ScopeSelection<K>;
  onSelect: (s: ScopeSelection<K>) => void;
  /** Overrides the default first-preset selection when the active kind changes. */
  onKindChange?: (kind: K) => void;
  presetsByKind: (kind: K) => ScopePreset[];
  savedPresets: ScopeSavedPreset<K>[];
  savedStatus?: ReactNode;
  onDeleteSaved: (id: string) => void;
  canSaveCurrent: boolean;
  onSaveCurrent: () => void;
} & SavedDefaultActionProps;

export function IntegrationScopeBar<K extends string>({
  className,
  testId,
  savedMenuTestId,
  kinds,
  selected,
  onSelect,
  onKindChange,
  presetsByKind,
  savedPresets,
  savedStatus,
  onDeleteSaved,
  canSaveCurrent,
  onSaveCurrent,
  onToggleSavedDefault,
  defaultMutationPendingId = null,
}: IntegrationScopeBarProps<K>) {
  const presets = presetsByKind(selected.kind);
  const saved = savedPresets.filter((p) => p.kind === selected.kind);
  const inbox = presets.filter((p) => p.group === "inbox");
  const created = presets.filter((p) => p.group === "created");

  const handleKindChange = useCallback(
    (kind: K) => {
      if (kind === selected.kind) return;
      if (onKindChange) {
        onKindChange(kind);
        return;
      }
      onSelect({ kind, source: "preset", id: presetsByKind(kind)[0]?.value ?? "" });
    },
    [onKindChange, onSelect, presetsByKind, selected.kind],
  );

  const renderPill = (p: ScopePreset) => (
    <PresetPill
      key={`${selected.kind}-${p.value}`}
      label={p.label}
      Icon={p.icon}
      iconName={p.iconName}
      active={selected.source === "preset" && selected.id === p.value}
      onClick={() => onSelect({ kind: selected.kind, source: "preset", id: p.value })}
    />
  );

  return (
    <div
      className={cn("flex items-center gap-1.5 overflow-x-auto px-4 py-2 sm:px-6", className)}
      data-testid={testId}
    >
      <KindSegment kinds={kinds} active={selected.kind} onChange={handleKindChange} />
      <Divider />
      {inbox.map(renderPill)}
      {inbox.length > 0 && created.length > 0 && <Divider />}
      {created.map(renderPill)}
      <div className="ml-auto shrink-0 pl-2">
        <SavedMenu
          testId={savedMenuTestId}
          selected={selected}
          saved={saved}
          savedStatus={savedStatus}
          onSelect={onSelect}
          onDeleteSaved={onDeleteSaved}
          canSaveCurrent={canSaveCurrent}
          onSaveCurrent={onSaveCurrent}
          onToggleSavedDefault={onToggleSavedDefault}
          defaultMutationPendingId={defaultMutationPendingId}
        />
      </div>
    </div>
  );
}
