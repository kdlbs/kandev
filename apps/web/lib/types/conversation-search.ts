import type { Task } from "./http";

export type ConversationMatch = {
  session_id: string;
  message_id: string;
  author_type: "user" | "agent";
  excerpt: string;
  highlight_ranges: [number, number][];
};
export type ConversationTask = Task & { conversation_match?: ConversationMatch };
export type ConversationSearchState =
  | "ready"
  | "searching"
  | "indexing"
  | "limited"
  | "unavailable"
  | "query_too_short"
  | "query_too_long"
  | "query_too_broad";
export type ConversationSearchResponse = {
  tasks: ConversationTask[];
  state: ConversationSearchState;
  has_more: boolean;
};
