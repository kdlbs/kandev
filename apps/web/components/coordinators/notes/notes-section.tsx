"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Skeleton } from "@kandev/ui/skeleton";
import { Switch } from "@kandev/ui/switch";
import { useNotes, type NotesLoadStatus } from "@/hooks/domains/coordinator/use-notes";
import {
  createNote,
  noteFailure,
  type NoteDraft,
  type NoteView,
  type NoteFailure,
} from "@/lib/api/domains/coordinator-notes-api";
import { emptyCitation } from "@/lib/coordinator/notes";
import { NoteDetail } from "./note-detail";
import { NoteEditor } from "./note-editor";
import { NoteList } from "./note-list";

type Props = { workspaceId: string; coordinatorId: string; canManage: boolean };

type Pane = { kind: "none" } | { kind: "note"; id: string } | { kind: "create"; draft: NoteDraft };

const BLANK_DRAFT: NoteDraft = { type: "project", text: "", citations: [emptyCitation()] };

function NotesListPane(props: {
  notes: NoteView[];
  status: NotesLoadStatus;
  selectedId: string | null;
  onSelect: (id: string) => void;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  const { notes, status, selectedId, onSelect, onRetry } = props;
  if (status === "loading") return <Skeleton className="h-4 w-64" data-testid="notes-loading" />;
  if (status === "error") {
    return (
      <div className="space-y-2" data-testid="notes-error">
        <p className="text-sm text-destructive">{t("coordinator:loadError")}</p>
        <Button
          type="button"
          variant="outline"
          className="min-h-11 cursor-pointer sm:min-h-8"
          onClick={onRetry}
        >
          {t("coordinator:retry")}
        </Button>
      </div>
    );
  }
  if (notes.length === 0) {
    return (
      <p className="text-sm text-muted-foreground" data-testid="notes-empty">
        {t("coordinator:notesEmpty")}
      </p>
    );
  }
  return <NoteList notes={notes} selectedId={selectedId} onSelect={onSelect} />;
}

/** The Notes section: the coordinator's advisory notes as a list with a detail pane. */
export function NotesSection({ workspaceId, coordinatorId, canManage }: Props) {
  const { t } = useTranslation();
  const [includeDeleted, setIncludeDeleted] = useState(false);
  const [pane, setPane] = useState<Pane>({ kind: "none" });
  const { notes, status, reload, retry } = useNotes(workspaceId, coordinatorId, includeDeleted);

  const create = async (draft: NoteDraft): Promise<NoteFailure | null> => {
    try {
      const made = await createNote(workspaceId, coordinatorId, draft);
      await reload();
      setPane({ kind: "note", id: made.id });
      return null;
    } catch (error) {
      return noteFailure(error);
    }
  };

  const open = pane.kind !== "none";
  return (
    <div className="space-y-3" data-testid="notes-section">
      <p className="text-sm text-muted-foreground">{t("coordinator:notesHelp")}</p>
      <div className={open ? "hidden md:flex" : "flex"}>
        <div className="flex w-full flex-wrap items-center justify-between gap-3">
          <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm sm:min-h-8">
            <Switch
              checked={includeDeleted}
              onCheckedChange={setIncludeDeleted}
              data-testid="notes-include-deleted"
            />
            {t("coordinator:notesIncludeDeleted")}
          </label>
          {canManage && (
            <Button
              type="button"
              className="min-h-11 cursor-pointer sm:min-h-8"
              data-testid="note-add"
              onClick={() => setPane({ kind: "create", draft: BLANK_DRAFT })}
            >
              {t("coordinator:noteAdd")}
            </Button>
          )}
        </div>
      </div>
      <div className="grid gap-4 md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        <div className={open ? "hidden md:block" : "block"}>
          <NotesListPane
            notes={notes}
            status={status}
            selectedId={pane.kind === "note" ? pane.id : null}
            onSelect={(id) => setPane({ kind: "note", id })}
            onRetry={retry}
          />
        </div>
        <div className={open ? "block" : "hidden md:block"} data-testid="notes-pane">
          {open && (
            <Button
              type="button"
              variant="ghost"
              className="mb-2 min-h-11 cursor-pointer md:hidden"
              data-testid="notes-back"
              onClick={() => setPane({ kind: "none" })}
            >
              {t("coordinator:notesBack")}
            </Button>
          )}
          {pane.kind === "create" && (
            <NoteEditor
              key="create"
              initial={pane.draft}
              onSubmit={create}
              onCancel={() => setPane({ kind: "none" })}
            />
          )}
          {pane.kind === "note" && (
            <NoteDetail
              key={pane.id}
              workspaceId={workspaceId}
              coordinatorId={coordinatorId}
              noteId={pane.id}
              canManage={canManage}
              onChanged={() => void reload()}
              onCreateFromDraft={(draft) => setPane({ kind: "create", draft })}
            />
          )}
        </div>
      </div>
    </div>
  );
}
