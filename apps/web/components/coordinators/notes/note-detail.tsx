"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Skeleton } from "@kandev/ui/skeleton";
import { useNoteDetail } from "@/hooks/domains/coordinator/use-notes";
import {
  deleteNote,
  editNote,
  noteFailure,
  revertNote,
  type NoteDetail as NoteDetailData,
  type NoteDraft,
  type NoteFailure,
} from "@/lib/api/domains/coordinator-notes-api";
import {
  citationSummary,
  formatNoteTime,
  noteFailureMessage,
  noteStatusLabel,
  noteTypeLabel,
} from "@/lib/coordinator/notes";
import { NoteEditor } from "./note-editor";
import { NoteReferences } from "./note-references";
import { NoteVersions } from "./note-versions";

type Props = {
  workspaceId: string;
  coordinatorId: string;
  noteId: string;
  canManage: boolean;
  /** Called after any accepted write so the list can reread. */
  onChanged: () => void;
  onCreateFromDraft: (draft: NoteDraft) => void;
};

function draftOf(detail: NoteDetailData): NoteDraft {
  return { type: detail.type, text: detail.text, citations: detail.citations };
}

function NoteBody({ detail }: { detail: NoteDetailData }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" data-testid="note-body">
      <p className="text-xs text-muted-foreground">
        {noteTypeLabel(t, detail.type)} · {noteStatusLabel(t, detail.status)} ·{" "}
        {t("coordinator:noteVersionNumber", { version: detail.version })}
      </p>
      <p className="whitespace-pre-wrap break-words text-sm">{detail.text}</p>
      <ul className="text-xs text-muted-foreground">
        {detail.citations.map((c, i) => (
          <li key={i} className="break-all">
            {citationSummary(c)}
          </li>
        ))}
      </ul>
      <p className="text-xs text-muted-foreground">
        {t("coordinator:noteExpiresAt", { time: formatNoteTime(detail.expires_at) })}
      </p>
    </div>
  );
}

function ActionError({ failure, onReload }: { failure: NoteFailure; onReload: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" role="alert" data-testid="note-action-error">
      <p className="text-sm text-destructive">{noteFailureMessage(t, failure)}</p>
      {failure.code === "conflict" && (
        <Button
          type="button"
          variant="outline"
          className="min-h-11 cursor-pointer sm:min-h-8"
          data-testid="note-action-reload"
          onClick={onReload}
        >
          {t("coordinator:noteConflictReload")}
        </Button>
      )}
    </div>
  );
}

function NoteActions(props: {
  canEdit: boolean;
  busy: boolean;
  confirming: boolean;
  onEdit: () => void;
  onAskDelete: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  const { canEdit, busy, confirming, onEdit, onAskDelete, onDelete } = props;
  return (
    <div className="flex flex-wrap gap-2">
      {canEdit && (
        <Button
          type="button"
          disabled={busy}
          className="min-h-11 cursor-pointer sm:min-h-8"
          data-testid="note-edit"
          onClick={onEdit}
        >
          {t("coordinator:noteEdit")}
        </Button>
      )}
      <Button
        type="button"
        variant={confirming ? "destructive" : "outline"}
        disabled={busy}
        className="min-h-11 cursor-pointer sm:min-h-8"
        data-testid={confirming ? "note-delete-confirm" : "note-delete"}
        onClick={confirming ? onDelete : onAskDelete}
      >
        {confirming ? t("coordinator:noteDeleteConfirm") : t("coordinator:noteDelete")}
      </Button>
    </div>
  );
}

/** One note: its stored state, versions and references, with edit, revert and delete. */
export function NoteDetail(props: Props) {
  const { workspaceId, coordinatorId, noteId, canManage, onChanged, onCreateFromDraft } = props;
  const { t } = useTranslation();
  const { status, detail, history, refresh } = useNoteDetail(workspaceId, coordinatorId, noteId);
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<NoteFailure | null>(null);

  if (status === "error") {
    return (
      <div className="space-y-2" data-testid="note-detail-error">
        <p className="text-sm text-destructive">{t("coordinator:loadError")}</p>
        <Button
          type="button"
          variant="outline"
          className="min-h-11 cursor-pointer sm:min-h-8"
          onClick={() => void refresh()}
        >
          {t("coordinator:retry")}
        </Button>
      </div>
    );
  }
  if (!detail) return <Skeleton className="h-4 w-64" data-testid="note-detail-loading" />;

  const expired = detail.status === "expired";
  const deleted = detail.status === "deleted";
  const mutable = canManage && !deleted;

  const run = async (call: () => Promise<unknown>) => {
    setBusy(true);
    setFailure(null);
    try {
      await call();
      setEditing(false);
      setConfirming(false);
      await refresh();
      onChanged();
    } catch (error) {
      setFailure(noteFailure(error));
    } finally {
      setBusy(false);
    }
  };
  const submitEdit = async (draft: NoteDraft): Promise<NoteFailure | null> => {
    try {
      await editNote(workspaceId, coordinatorId, noteId, { ...draft, version: detail.version });
      setEditing(false);
      await refresh();
      onChanged();
      return null;
    } catch (error) {
      return noteFailure(error);
    }
  };

  if (editing) {
    return (
      <NoteEditor
        initial={draftOf(detail)}
        onSubmit={submitEdit}
        onCancel={() => setEditing(false)}
        onReload={refresh}
        onCreateFromDraft={onCreateFromDraft}
      />
    );
  }
  return (
    <div className="space-y-4" data-testid="note-detail">
      <NoteBody detail={detail} />
      {failure && <ActionError failure={failure} onReload={() => void refresh()} />}
      {mutable && (
        <NoteActions
          canEdit={!expired}
          busy={busy}
          confirming={confirming}
          onEdit={() => setEditing(true)}
          onAskDelete={() => setConfirming(true)}
          onDelete={() =>
            void run(() => deleteNote(workspaceId, coordinatorId, noteId, detail.version))
          }
        />
      )}
      <NoteVersions
        versions={detail.versions}
        currentVersion={detail.version}
        canRevert={mutable && !expired}
        busy={busy}
        onRevert={(target) =>
          void run(() =>
            revertNote(workspaceId, coordinatorId, noteId, {
              target_version: target,
              version: detail.version,
            }),
          )
        }
      />
      <NoteReferences referrers={detail.referenced_by} history={history} />
    </div>
  );
}
