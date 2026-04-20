"use client";

import { useState } from "react";
import { Input } from "@kandev/ui/input";
import { Button } from "@kandev/ui/button";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";

type HeaderMode = "view" | "rename" | "saveAs";

export function ViewHeaderRow({
  activeView,
  hasDraft,
  canDelete,
  onSaveOverwrite,
  onSaveAs,
  onRename,
  onDiscard,
  onDelete,
}: {
  activeView: SidebarView | undefined;
  hasDraft: boolean;
  canDelete: boolean;
  onSaveOverwrite: () => void;
  onSaveAs: (name: string) => void;
  onRename: (id: string, name: string) => void;
  onDiscard: () => void;
  onDelete: () => void;
}) {
  const [mode, setMode] = useState<HeaderMode>("view");
  const [nameDraft, setNameDraft] = useState("");

  const isEditing = mode !== "view";
  const canOverwrite = hasDraft && !!activeView;

  function enterRename() {
    if (!activeView) return;
    setNameDraft(activeView.name);
    setMode("rename");
  }

  function enterSaveAs() {
    setNameDraft("");
    setMode("saveAs");
  }

  function exit() {
    setMode("view");
    setNameDraft("");
  }

  function submit() {
    const trimmed = nameDraft.trim();
    if (!trimmed) return;
    if (mode === "rename" && activeView) {
      onRename(activeView.id, trimmed);
    } else if (mode === "saveAs") {
      onSaveAs(trimmed);
    }
    exit();
  }

  return (
    <div className="flex items-center justify-between gap-2">
      <div className="flex flex-1 items-center gap-2 text-xs">
        <span className="text-muted-foreground">
          {mode === "saveAs" ? "Save as:" : "View:"}
        </span>
        {isEditing ? (
          <Input
            autoFocus
            value={nameDraft}
            onChange={(e) => setNameDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") submit();
              if (e.key === "Escape") exit();
            }}
            placeholder={mode === "saveAs" ? "New view name" : undefined}
            className="h-6 flex-1 text-xs"
            data-testid={
              mode === "rename" ? "view-rename-input" : "view-save-as-name-input"
            }
          />
        ) : (
          <>
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
          </>
        )}
      </div>
      <div className="flex items-center gap-1">
        {isEditing ? (
          <>
            <Button
              type="button"
              size="sm"
              className="h-6 cursor-pointer text-xs"
              onClick={submit}
              disabled={!nameDraft.trim()}
              data-testid={
                mode === "rename" ? "view-rename-confirm" : "view-save-as-confirm"
              }
            >
              {mode === "rename" ? "Save" : "Create"}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="h-6 cursor-pointer text-xs"
              onClick={exit}
            >
              Cancel
            </Button>
          </>
        ) : (
          <>
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
                variant="outline"
                className="h-6 cursor-pointer text-xs"
                onClick={enterSaveAs}
                data-testid="view-save-as-button"
              >
                Save as…
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
            {!hasDraft && activeView && (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="h-6 cursor-pointer text-xs"
                onClick={enterRename}
                data-testid="view-rename-button"
              >
                Rename
              </Button>
            )}
            {!hasDraft && activeView && canDelete && (
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
          </>
        )}
      </div>
    </div>
  );
}
