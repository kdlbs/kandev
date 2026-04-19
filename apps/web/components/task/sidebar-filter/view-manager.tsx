"use client";

import { useState } from "react";
import { Input } from "@kandev/ui/input";
import { Button } from "@kandev/ui/button";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";

export function ViewHeaderRow({
  activeView,
  hasDraft,
  canDelete,
  onSaveOverwrite,
  onDiscard,
  onDelete,
}: {
  activeView: SidebarView | undefined;
  hasDraft: boolean;
  canDelete: boolean;
  onSaveOverwrite: () => void;
  onDiscard: () => void;
  onDelete: () => void;
}) {
  const canOverwrite = hasDraft && !!activeView;
  return (
    <div className="mb-2 flex items-center justify-between gap-2">
      <div className="flex items-center gap-2 text-xs">
        <span className="text-muted-foreground">View:</span>
        <span className="font-medium" data-testid="sidebar-filter-active-view-name">
          {activeView?.name ?? "—"}
        </span>
        {hasDraft && (
          <span
            className="h-1.5 w-1.5 rounded-full bg-amber-500"
            data-testid="sidebar-filter-dirty-indicator"
            title="Unsaved changes"
          />
        )}
      </div>
      <div className="flex items-center gap-1">
        {canOverwrite && (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="h-6 cursor-pointer text-xs"
            onClick={onSaveOverwrite}
            data-testid="view-save-button"
          >
            Save
          </Button>
        )}
        {hasDraft && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="h-6 cursor-pointer text-xs"
            onClick={onDiscard}
            data-testid="view-discard-button"
          >
            Discard
          </Button>
        )}
        {activeView && canDelete && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="h-6 cursor-pointer text-xs text-destructive"
            onClick={onDelete}
            data-testid="view-delete-button"
          >
            Delete
          </Button>
        )}
      </div>
    </div>
  );
}

export function ViewSaveAsRow({ onSubmit }: { onSubmit: (name: string) => void }) {
  const [showSaveAs, setShowSaveAs] = useState(false);
  const [name, setName] = useState("");

  function submit() {
    if (!name.trim()) return;
    onSubmit(name.trim());
    setName("");
    setShowSaveAs(false);
  }

  if (!showSaveAs) {
    return (
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="h-7 w-full cursor-pointer text-xs"
        onClick={() => setShowSaveAs(true)}
        data-testid="view-save-as-button"
      >
        Save as new view…
      </Button>
    );
  }

  return (
    <div className="flex items-center gap-1.5">
      <Input
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") submit();
          if (e.key === "Escape") {
            setName("");
            setShowSaveAs(false);
          }
        }}
        placeholder="View name"
        className="h-7 flex-1 text-xs"
        data-testid="view-save-as-name-input"
      />
      <Button
        type="button"
        size="sm"
        className="h-7 cursor-pointer text-xs"
        onClick={submit}
        disabled={!name.trim()}
        data-testid="view-save-as-confirm"
      >
        Create
      </Button>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className="h-7 cursor-pointer text-xs"
        onClick={() => {
          setName("");
          setShowSaveAs(false);
        }}
      >
        Cancel
      </Button>
    </div>
  );
}

export function ViewRenameRow({
  view,
  onRename,
}: {
  view: SidebarView;
  onRename: (id: string, name: string) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(view.name);

  if (!editing) {
    return (
      <button
        type="button"
        onClick={() => {
          setName(view.name);
          setEditing(true);
        }}
        className="mt-1 cursor-pointer text-[11px] text-muted-foreground hover:text-foreground"
        data-testid="view-rename-button"
      >
        Rename view
      </button>
    );
  }
  return (
    <div className="mt-1 flex items-center gap-1.5">
      <Input
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            onRename(view.id, name);
            setEditing(false);
          }
          if (e.key === "Escape") setEditing(false);
        }}
        className="h-7 flex-1 text-xs"
        data-testid="view-rename-input"
      />
      <Button
        type="button"
        size="sm"
        className="h-7 cursor-pointer text-xs"
        onClick={() => {
          onRename(view.id, name);
          setEditing(false);
        }}
      >
        Save
      </Button>
    </div>
  );
}
