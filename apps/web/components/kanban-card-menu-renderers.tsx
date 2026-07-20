"use client";

import {
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import {
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from "@kandev/ui/dropdown-menu";
import type { KanbanCardMenuEntry } from "./kanban-card-menu-items";

function ContextEntry({ entry }: { entry: KanbanCardMenuEntry }) {
  if (entry.kind === "separator") return <ContextMenuSeparator />;
  if (entry.kind === "submenu") {
    return (
      <ContextMenuSub>
        <ContextMenuSubTrigger data-testid={entry.testId} disabled={entry.disabled}>
          {entry.icon}
          {entry.label}
        </ContextMenuSubTrigger>
        <ContextMenuSubContent className={entry.className}>
          {entry.children.map((child) => (
            <ContextEntry key={child.key} entry={child} />
          ))}
        </ContextMenuSubContent>
      </ContextMenuSub>
    );
  }

  return (
    <ContextMenuItem
      data-testid={entry.testId}
      disabled={entry.disabled}
      className={entry.destructive ? "text-destructive focus:text-destructive" : undefined}
      onClick={(event) => event.stopPropagation()}
      onSelect={() => {
        if (!entry.disabled) entry.onSelect?.();
      }}
    >
      {entry.icon}
      {entry.leading}
      {entry.label}
      {entry.trailing}
    </ContextMenuItem>
  );
}

function DropdownEntry({ entry }: { entry: KanbanCardMenuEntry }) {
  if (entry.kind === "separator") return <DropdownMenuSeparator />;
  if (entry.kind === "submenu") {
    return (
      <DropdownMenuSub>
        <DropdownMenuSubTrigger
          data-testid={entry.testId}
          disabled={entry.disabled}
          onClick={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
        >
          {entry.icon}
          {entry.label}
        </DropdownMenuSubTrigger>
        <DropdownMenuPortal>
          <DropdownMenuSubContent className={entry.className}>
            {entry.children.map((child) => (
              <DropdownEntry key={child.key} entry={child} />
            ))}
          </DropdownMenuSubContent>
        </DropdownMenuPortal>
      </DropdownMenuSub>
    );
  }

  return (
    <DropdownMenuItem
      data-testid={entry.testId}
      disabled={entry.disabled}
      className={entry.destructive ? "text-destructive focus:text-destructive" : undefined}
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onSelect={(event) => {
        event.stopPropagation();
        if (!entry.disabled) entry.onSelect?.();
      }}
    >
      {entry.icon}
      {entry.leading}
      {entry.label}
      {entry.trailing}
    </DropdownMenuItem>
  );
}

export function KanbanCardContextMenuItems({ entries }: { entries: KanbanCardMenuEntry[] }) {
  return entries.map((entry) => <ContextEntry key={entry.key} entry={entry} />);
}

export function KanbanCardDropdownMenuItems({ entries }: { entries: KanbanCardMenuEntry[] }) {
  return entries.map((entry) => <DropdownEntry key={entry.key} entry={entry} />);
}
