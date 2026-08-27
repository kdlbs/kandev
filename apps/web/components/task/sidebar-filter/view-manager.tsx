"use client";

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Input } from "@kandev/ui/input";
import { Button } from "@kandev/ui/button";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import { useTranslation } from "react-i18next";
import { sidebarViewName } from "@/lib/state/slices/ui/sidebar-view-builtins";

type HeaderMode = "view" | "rename" | "saveAs";

type HeaderProps = {
  activeView: SidebarView | undefined;
  hasDraft: boolean;
  canDelete: boolean;
  onSaveOverwrite: () => void;
  onSaveAs: (name: string) => void;
  onRename: (id: string, name: string) => void;
  onDiscard: () => void;
  onDelete: () => void;
  renameRequestedViewId?: string | null;
  onRenameRequestHandled?: (viewId: string) => void;
};

export function ViewHeaderRow(props: HeaderProps) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<HeaderMode>("view");
  const [nameDraft, setNameDraft] = useState("");
  const [editingViewId, setEditingViewId] = useState<string | null>(null);
  const activeViewId = props.activeView?.id;
  const activeViewName = props.activeView?.name;
  const isEditing = mode !== "view";

  function enterRename() {
    if (!props.activeView) return;
    setNameDraft(props.activeView.name);
    setEditingViewId(props.activeView.id);
    setMode("rename");
  }
  function enterSaveAs() {
    setNameDraft("");
    setMode("saveAs");
  }
  function exit() {
    setMode("view");
    setNameDraft("");
    setEditingViewId(null);
  }
  function submit() {
    const trimmed = nameDraft.trim();
    if (!trimmed) return;
    if (mode === "rename" && props.activeView) props.onRename(props.activeView.id, trimmed);
    else if (mode === "saveAs") props.onSaveAs(trimmed);
    exit();
  }

  useLayoutEffect(() => {
    const requestedViewId = props.renameRequestedViewId;
    if (!requestedViewId || activeViewId !== requestedViewId || activeViewName === undefined)
      return;
    setNameDraft(activeViewName);
    setEditingViewId(requestedViewId);
    setMode("rename");
    props.onRenameRequestHandled?.(requestedViewId);
  }, [activeViewId, activeViewName, props.onRenameRequestHandled, props.renameRequestedViewId]);

  useLayoutEffect(() => {
    if (mode !== "rename" || !editingViewId || activeViewId === editingViewId) return;
    setMode("view");
    setNameDraft("");
    setEditingViewId(null);
  }, [activeViewId, editingViewId, mode]);

  return (
    <div className="flex items-center justify-between gap-2">
      <div className="flex flex-1 items-center gap-2 text-xs">
        <span className="text-muted-foreground">
          {mode === "saveAs" ? t("task:saveAs") : t("task:view")}
        </span>
        {isEditing ? (
          <NameInput
            mode={mode}
            value={nameDraft}
            onChange={setNameDraft}
            onSubmit={submit}
            onCancel={exit}
          />
        ) : (
          <NameDisplay activeView={props.activeView} hasDraft={props.hasDraft} />
        )}
      </div>
      <div className="flex items-center gap-1">
        {isEditing ? (
          <EditingActions
            mode={mode}
            canSubmit={!!nameDraft.trim()}
            onSubmit={submit}
            onCancel={exit}
          />
        ) : (
          <ViewActions {...props} onRename={enterRename} onSaveAs={enterSaveAs} />
        )}
      </div>
    </div>
  );
}

function NameDisplay({
  activeView,
  hasDraft,
}: {
  activeView: SidebarView | undefined;
  hasDraft: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <span className="font-medium" data-testid="sidebar-filter-active-view-name">
        {activeView ? sidebarViewName(activeView, t) : t("task:none")}
      </span>
      {hasDraft && (
        <span
          className="h-1.5 w-1.5 rounded-full bg-amber-500"
          data-testid="sidebar-filter-dirty-indicator"
          title={t("common:unsavedChanges")}
        />
      )}
    </>
  );
}

function NameInput({
  mode,
  value,
  onChange,
  onSubmit,
  onCancel,
}: {
  mode: HeaderMode;
  value: string;
  onChange: (v: string) => void;
  onSubmit: () => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);

  useLayoutEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      inputRef.current?.focus();
      inputRef.current?.select();
    });
    return () => cancelAnimationFrame(frame);
  }, []);

  return (
    <Input
      ref={inputRef}
      autoFocus
      aria-label={mode === "rename" ? t("task:viewName") : t("task:newViewName")}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === "Enter") onSubmit();
        if (e.key === "Escape") onCancel();
      }}
      placeholder={mode === "saveAs" ? t("task:newViewName") : undefined}
      className="h-6 flex-1 text-xs"
      data-testid={mode === "rename" ? "view-rename-input" : "view-save-as-name-input"}
    />
  );
}

function EditingActions({
  mode,
  canSubmit,
  onSubmit,
  onCancel,
}: {
  mode: HeaderMode;
  canSubmit: boolean;
  onSubmit: () => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Button
        type="button"
        size="sm"
        className="h-6 cursor-pointer text-xs"
        onClick={onSubmit}
        disabled={!canSubmit}
        data-testid={mode === "rename" ? "view-rename-confirm" : "view-save-as-confirm"}
      >
        {mode === "rename" ? t("common:save") : t("task:create")}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className="h-6 cursor-pointer text-xs"
        onClick={onCancel}
      >
        {t("common:cancel")}
      </Button>
    </>
  );
}

function ViewActions({
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
  onSaveAs: () => void;
  onRename: () => void;
  onDiscard: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  const canOverwrite = hasDraft && !!activeView;
  return (
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
          {t("common:save")}
        </Button>
      )}
      {hasDraft && (
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-6 cursor-pointer text-xs"
          onClick={onSaveAs}
          data-testid="view-save-as-button"
        >
          {t("task:saveAs2")}
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
          {t("task:discard")}
        </Button>
      )}
      {!hasDraft && activeView && (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="h-6 cursor-pointer text-xs"
          onClick={onRename}
          data-testid="view-rename-button"
        >
          {t("task:rename")}
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
          {t("task:delete")}
        </Button>
      )}
    </>
  );
}
