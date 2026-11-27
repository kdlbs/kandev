import { useId, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import { Drawer, DrawerContent, DrawerTitle } from "@kandev/ui/drawer";
import { Textarea } from "@kandev/ui/textarea";
import { TASK_FLAG_NOTE_MAX_LENGTH } from "@/lib/task-flag-note";

export function TaskFlagEditor({
  savedNote,
  readOnly,
  touch,
  onSubmit,
  onClose,
  onCloseAutoFocus,
}: {
  savedNote: string;
  readOnly: boolean;
  touch: boolean;
  onSubmit: (note: string) => Promise<void>;
  onClose: () => void;
  onCloseAutoFocus: (event: Event) => void;
}) {
  const { t } = useTranslation();
  const draft = useTaskFlagDraft(savedNote, onSubmit, onClose);
  const title = readOnly
    ? t("sidebar:flag")
    : t(savedNote ? "sidebar:editFlag" : "sidebar:addFlag");
  const form = readOnly ? (
    <div className="grid gap-3">
      <p className="whitespace-pre-wrap wrap-anywhere">{savedNote}</p>
      <Button className="ml-auto min-h-11" variant="outline" onClick={draft.close}>
        {t("common:close")}
      </Button>
    </div>
  ) : (
    <TaskFlagForm draft={draft} touch={touch} />
  );

  if (touch) {
    return (
      <Drawer open onOpenChange={draft.close} dismissible={!draft.saving}>
        <DrawerContent
          className="data-[vaul-drawer-direction=bottom]:max-h-[80dvh]"
          aria-describedby={undefined}
          onCloseAutoFocus={onCloseAutoFocus}
        >
          <div className="min-h-0 overflow-y-auto overscroll-contain px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
            <DrawerTitle className="mb-3 text-base">{title}</DrawerTitle>
            {form}
          </div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open onOpenChange={draft.close}>
      <DialogContent
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto overscroll-contain sm:max-w-sm"
        aria-describedby={undefined}
        showCloseButton={false}
        enterConfirms={false}
        onCloseAutoFocus={onCloseAutoFocus}
      >
        <DialogTitle>{title}</DialogTitle>
        {form}
      </DialogContent>
    </Dialog>
  );
}

function useTaskFlagDraft(
  savedNote: string,
  onSubmit: (note: string) => Promise<void>,
  onClose: () => void,
) {
  const [note, setNote] = useState(savedNote);
  const [failed, setFailed] = useState(false);
  const [saving, setSaving] = useState(false);
  const submitting = useRef(false);
  const trimmedNote = note.trim();
  const canSave = Boolean(trimmedNote) && note.length <= TASK_FLAG_NOTE_MAX_LENGTH;

  const close = () => {
    if (!submitting.current) onClose();
  };
  const submit = async (next: string) => {
    if (submitting.current) return;
    submitting.current = true;
    setSaving(true);
    setFailed(false);
    try {
      await onSubmit(next);
      onClose();
    } catch {
      setFailed(true);
    } finally {
      submitting.current = false;
      setSaving(false);
    }
  };
  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (canSave) void submit(trimmedNote);
  };

  return { note, setNote, failed, saving, canSave, close, handleSubmit };
}

function TaskFlagForm({
  draft: { note, setNote, failed, saving, canSave, close, handleSubmit },
  touch = false,
}: {
  draft: ReturnType<typeof useTaskFlagDraft>;
  touch?: boolean;
}) {
  const { t } = useTranslation();
  const noteId = useId();
  const buttonSize = touch ? "min-h-11" : undefined;
  return (
    <form className="grid gap-3" onSubmit={handleSubmit}>
      <div className="grid gap-1.5">
        <label htmlFor={noteId} className="text-sm font-medium">
          {t("sidebar:flagNote")}
        </label>
        <Textarea
          id={noteId}
          value={note}
          rows={3}
          maxLength={TASK_FLAG_NOTE_MAX_LENGTH}
          className={
            touch ? "md:text-base" : "max-h-[min(16rem,40dvh)] overflow-y-auto overscroll-contain"
          }
          readOnly={saving}
          onChange={(event) => setNote(event.target.value)}
        />
      </div>
      <span role="status" className="sr-only">
        {saving && t("task:saving")}
      </span>
      {failed && (
        <p role="alert" className="text-destructive">
          {t("sidebar:flagSaveFailed")}
        </p>
      )}
      <div className="ml-auto flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          className={buttonSize}
          disabled={saving}
          onClick={close}
        >
          {t("common:cancel")}
        </Button>
        <Button type="submit" className={buttonSize} disabled={saving || !canSave}>
          {t("common:save")}
        </Button>
      </div>
    </form>
  );
}
