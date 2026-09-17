import { createContext } from "react";

export type CommentDraftStore = {
  get: (taskId: string) => string;
  set: (taskId: string, value: string) => void;
};

// Optional memory-only drafts supplied by persistent conversation surfaces.
export const CommentDraftContext = createContext<CommentDraftStore | null>(null);
