import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";

type NoteUpdatedMessage = BackendMessageMap["task.note.updated"];
type NoteDeletedMessage = BackendMessageMap["task.note.deleted"];

function handleNoteUpsert(store: StoreApi<AppState>, message: NoteUpdatedMessage) {
  const { task_id, id, content, updated_by, created_at, updated_at } = message.payload;
  store.getState().setTaskNote(task_id, {
    id,
    task_id,
    content,
    updated_by,
    created_at,
    updated_at,
  });
}

function handleNoteDeleted(store: StoreApi<AppState>, message: NoteDeletedMessage) {
  store.getState().clearTaskNote(message.payload.task_id);
}

export function registerTaskNotesHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.note.updated": (message) => handleNoteUpsert(store, message),
    "task.note.deleted": (message) => handleNoteDeleted(store, message),
  };
}
