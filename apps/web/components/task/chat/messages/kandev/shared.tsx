"use client";

import { createContext, useContext, type ReactNode } from "react";
import { Badge } from "@kandev/ui/badge";
import {
  IconCheck,
  IconClock,
  IconX,
  type IconProps,
  type Icon as TablerIcon,
} from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { cn } from "@/lib/utils";
import { ExpandableRow } from "../expandable-row";
import { useExpandState } from "../use-expand-state";
import { shortId } from "./parse";

export type KandevStatus = "pending" | "running" | "complete" | "error" | undefined;

export function KandevStatusIcon({ status }: { status: KandevStatus }) {
  if (status === "complete") return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
  if (status === "error") return <IconX className="h-3.5 w-3.5 text-red-500" />;
  if (status === "running") return <GridSpinner className="text-muted-foreground" />;
  return null;
}

// KandevPermissionUIState is the *display overlay* applied to a Kandev row
// while a permission_request message is attached. It deliberately does NOT
// override the tool_call's own `status`: a tool can be approved but still
// pending, or rejected before it ever ran, and the underlying tool state is
// the source of truth for things like the green check, spinner, and red X.
// The overlay only drives "waiting on the user" affordances (amber clock,
// hidden summary) and the rejection mark.
export type KandevPermissionUIState = "pending" | "rejected" | undefined;

const KandevPermissionUIContext = createContext<KandevPermissionUIState>(undefined);

export const KandevPermissionUIProvider = KandevPermissionUIContext.Provider;

type KandevRowProps = {
  Icon: TablerIcon | ((p: IconProps) => ReactNode);
  title: string;
  summary?: ReactNode;
  status: KandevStatus;
  hasExpandableContent: boolean;
  children?: ReactNode;
};

// KandevRow is the consistent header row for every Kandev tool. Title is the
// "Kandev: …" string, summary is an inline description of args / result count,
// and children are the expanded body. Auto-expands while the tool is running,
// matching the pattern used by ToolExecuteMessage. The permission overlay
// (amber clock / hidden summary / red X) is layered on top of the underlying
// tool status via `KandevPermissionUIContext` so the row never lies about the
// real tool_call state.
export function KandevRow({
  Icon,
  title,
  summary,
  status,
  hasExpandableContent,
  children,
}: KandevRowProps) {
  const permissionUI = useContext(KandevPermissionUIContext);
  const isAwaitingPermission = permissionUI === "pending";
  const isRejected = permissionUI === "rejected";
  const autoExpanded = status === "running";
  const { isExpanded, handleToggle } = useExpandState(status, autoExpanded);
  // The header chip on the right shows one of: amber clock (waiting on the
  // user), red X (rejected, takes precedence over the tool's own state),
  // otherwise the tool's own status icon. We suppress that chip on success
  // because the leftmost icon already conveys "done"; rejection wins over the
  // success short-circuit so a tool that completed before the user denied it
  // still reads as denied.
  const isSuccess = status === "complete" && !isRejected;

  return (
    <ExpandableRow
      icon={<Icon className="h-4 w-4 text-muted-foreground" />}
      header={
        <div className="flex items-center gap-2 text-xs min-w-0">
          <span className="font-mono text-xs text-muted-foreground shrink-0">{title}</span>
          {summary && !isAwaitingPermission && (
            <span className="text-xs text-muted-foreground/80 truncate min-w-0">{summary}</span>
          )}
          {!isSuccess && (
            <span className="shrink-0">
              {renderHeaderStatusIcon({ isAwaitingPermission, isRejected, status })}
            </span>
          )}
        </div>
      }
      hasExpandableContent={hasExpandableContent}
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      {children}
    </ExpandableRow>
  );
}

function renderHeaderStatusIcon({
  isAwaitingPermission,
  isRejected,
  status,
}: {
  isAwaitingPermission: boolean;
  isRejected: boolean;
  status: KandevStatus;
}) {
  if (isAwaitingPermission)
    return <IconClock className="h-3.5 w-3.5 text-amber-600 dark:text-amber-400" />;
  if (isRejected) return <IconX className="h-3.5 w-3.5 text-red-500" />;
  return <KandevStatusIcon status={status} />;
}

// KandevBody is the bordered container used by all expanded sections so they
// match the look of ToolCallExpandedContent / ToolExecuteContent.
export function KandevBody({ children }: { children: ReactNode }) {
  return <div className="pl-4 border-l-2 border-border/30 space-y-2">{children}</div>;
}

// SummaryDot is a separator we use to chain short summary fragments in the
// header (`workflow=abc · 3 tasks`). The space-flanked middle-dot keeps the
// line compact even on narrow viewports.
export function SummaryDot() {
  return <span className="text-muted-foreground/40">·</span>;
}

// IdChip displays a UUID-style id in a very subtle inline form, with the
// full id available on hover. No `label=` prefix — surrounding context
// (column position, neighbouring text) carries the meaning, and the prefix
// was making rows visually noisy.
export function IdChip({ id }: { id: string | undefined }) {
  if (!id) return null;
  return (
    <span className="font-mono text-[10px] text-muted-foreground/50" title={id}>
      {shortId(id)}
    </span>
  );
}

// KeyValueRow renders a single labelled field inside the expanded body. Used
// by per-tool renderers to format arg/result fields without rebuilding the
// same flex/gap markup over and over.
export function KeyValueRow({
  label,
  children,
  mono,
}: {
  label: string;
  children: ReactNode;
  mono?: boolean;
}) {
  return (
    <div className="flex items-baseline gap-2 text-xs">
      <span className="text-muted-foreground/70 shrink-0">{label}</span>
      <span className={cn("text-foreground break-words min-w-0", mono && "font-mono text-[11px]")}>
        {children}
      </span>
    </div>
  );
}

// TaskStateBadge maps backend task states to coloured shadcn badges. Unknown
// states still render — they fall through to the outline variant so a new
// state added on the backend doesn't disappear from the UI before the
// frontend is updated.
const TASK_STATE_VARIANTS: Record<
  string,
  { variant: "default" | "secondary" | "outline" | "destructive"; className?: string }
> = {
  CREATED: { variant: "outline" },
  SCHEDULING: { variant: "secondary" },
  RUNNING: { variant: "default" },
  WAITING_FOR_INPUT: { variant: "secondary" },
  PAUSED: { variant: "secondary" },
  COMPLETED: { variant: "default", className: "bg-green-600 hover:bg-green-600" },
  FAILED: { variant: "destructive" },
  CANCELLED: { variant: "outline" },
  ARCHIVED: { variant: "outline" },
};

export function TaskStateBadge({ state }: { state: string | undefined }) {
  if (!state) return null;
  const cfg = TASK_STATE_VARIANTS[state] ?? { variant: "outline" as const };
  return (
    <Badge variant={cfg.variant} className={cn("uppercase tracking-wide", cfg.className)}>
      {state}
    </Badge>
  );
}

// EmptyListNote is what we render when a list response comes back with 0
// items. Plain "No results" rendered inside the body rather than the header so
// the header stays consistent across all states.
export function EmptyListNote({ noun }: { noun: string }) {
  return <div className="text-xs text-muted-foreground italic">No {noun} found.</div>;
}

// ListItemRow is one row in a list-style result. Plain layout — no border,
// no background — so a long list reads as a clean column rather than a
// stack of nested cards.
export function ListItemRow({ children }: { children: ReactNode }) {
  return <div className="text-xs space-y-0.5">{children}</div>;
}

// CountBadge formats `N items` / `1 item` for the header summary. Pure helper
// kept here so all renderers pluralise the same way.
export function pluralCount(n: number, noun: string, nounPlural?: string): string {
  if (n === 1) return `1 ${noun}`;
  return `${n} ${nounPlural ?? `${noun}s`}`;
}
