"use client";

import { useEffect, useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Drawer, DrawerContent, DrawerFooter, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { renameCanvas, type Canvas } from "@/lib/api/domains/canvas-api";

export function CanvasRenameDialog({
  canvas,
  open,
  onOpenChange,
  onRenamed,
}: {
  canvas: Canvas | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onRenamed: () => void;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [draft, setDraft] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (!open) return;
    setDraft(canvas?.title ?? "");
    setError("");
    const timer = window.setTimeout(() => inputRef.current?.select(), 0);
    return () => window.clearTimeout(timer);
  }, [canvas?.id, open]);
  const save = async () => {
    if (!canvas || saving) return;
    const title = draft.trim();
    if (!title || Array.from(title).length > 200) {
      setError(t("canvases:renameCanvasInvalid"));
      inputRef.current?.focus();
      return;
    }
    setSaving(true);
    setError("");
    try {
      await renameCanvas(canvas.id, title);
      onOpenChange(false);
      onRenamed();
    } catch {
      setError(t("canvases:renameCanvasFailed"));
      inputRef.current?.focus();
    } finally {
      setSaving(false);
    }
  };
  const content = (
    <RenameForm
      draft={draft}
      onDraftChange={setDraft}
      error={error}
      saving={saving}
      inputRef={inputRef}
      onSave={() => void save()}
      onCancel={() => onOpenChange(false)}
    />
  );
  if (isMobile)
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent className="pb-[env(safe-area-inset-bottom)]">
          <DrawerHeader>
            <DrawerTitle>{t("canvases:renameCanvas")}</DrawerTitle>
          </DrawerHeader>
          {content}
          <DrawerFooter className="hidden" />
        </DrawerContent>
      </Drawer>
    );
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("canvases:renameCanvas")}</DialogTitle>
        </DialogHeader>
        {content}
        <DialogFooter className="hidden" />
      </DialogContent>
    </Dialog>
  );
}

function RenameForm({
  draft,
  onDraftChange,
  error,
  saving,
  inputRef,
  onSave,
  onCancel,
}: {
  draft: string;
  onDraftChange: (value: string) => void;
  error: string;
  saving: boolean;
  inputRef: RefObject<HTMLInputElement | null>;
  onSave: () => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        onSave();
      }}
      className="space-y-3 px-4 pb-4"
    >
      <Label htmlFor="canvas-rename-input">{t("canvases:canvasName")}</Label>
      <Input
        id="canvas-rename-input"
        ref={inputRef}
        value={draft}
        onChange={(event) => onDraftChange(Array.from(event.target.value).slice(0, 200).join(""))}
        aria-invalid={Boolean(error)}
        className="min-h-11"
      />
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <div className="flex gap-2 justify-end">
        <Button type="button" variant="outline" className="min-h-11" onClick={onCancel}>
          {t("common:cancel")}
        </Button>
        <Button type="submit" className="min-h-11" disabled={saving}>
          {t("common:save")}
        </Button>
        <span role="status" className="sr-only">
          {saving ? t("canvases:renamingCanvas") : ""}
        </span>
      </div>
    </form>
  );
}
