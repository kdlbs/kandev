"use client";

import { useTranslation } from "react-i18next";
import type { NoteView } from "@/lib/api/domains/coordinator-notes-api";
import { noteStatusLabel, noteTypeLabel } from "@/lib/coordinator/notes";

type Props = { notes: NoteView[]; selectedId: string | null; onSelect: (id: string) => void };

export function NoteList({ notes, selectedId, onSelect }: Props) {
  const { t } = useTranslation();
  return (
    <ul className="space-y-1" data-testid="note-list">
      {notes.map((note) => (
        <li key={note.id}>
          <button
            type="button"
            aria-current={note.id === selectedId || undefined}
            className="flex min-h-12 w-full cursor-pointer flex-col items-start gap-0.5 rounded-md border p-2 text-left text-sm hover:bg-accent aria-[current]:bg-accent sm:min-h-9"
            data-testid={`note-row-${note.id}`}
            onClick={() => onSelect(note.id)}
          >
            <span className="line-clamp-2 break-words">{note.text}</span>
            <span className="text-xs text-muted-foreground">
              {noteTypeLabel(t, note.type)} · {noteStatusLabel(t, note.status)}
            </span>
          </button>
        </li>
      ))}
    </ul>
  );
}
