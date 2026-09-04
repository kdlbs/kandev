"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { MAX_THREAD_VIEWS } from "@/lib/state/slices/ui/thread-view-builtins";

export function EditorActions({
  hasDraft,
  canDelete,
  viewCount,
  invalidDraft,
  onSave,
  onSaveAs,
  onDiscard,
  onDelete,
}: {
  hasDraft: boolean;
  canDelete: boolean;
  viewCount: number;
  invalidDraft: boolean;
  onSave: () => void;
  onSaveAs: () => void;
  onDiscard: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center justify-end gap-1 p-2">
      {hasDraft && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-8 cursor-pointer text-xs"
          onClick={onSave}
          disabled={invalidDraft}
          data-testid="threads-view-save"
        >
          {t("common:save")}
        </Button>
      )}
      {hasDraft && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-8 cursor-pointer text-xs"
          onClick={onSaveAs}
          disabled={viewCount >= MAX_THREAD_VIEWS || invalidDraft}
          data-testid="threads-view-save-as"
        >
          {t("threads:saveAs")}
        </Button>
      )}
      {hasDraft && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-8 cursor-pointer text-xs"
          onClick={onDiscard}
          data-testid="threads-view-discard"
        >
          {t("task:discard")}
        </Button>
      )}
      {!hasDraft && canDelete && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-8 cursor-pointer text-xs text-destructive"
          onClick={onDelete}
          data-testid="threads-view-delete"
        >
          {t("task:delete")}
        </Button>
      )}
    </div>
  );
}

export function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
      {children}
    </div>
  );
}
