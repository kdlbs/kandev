import { createContext } from "react";
export type CommentTransport = (
  taskId: string,
  body: { body: string; author_type: string },
) => Promise<unknown>;
// Feature shells supply transport; the shared chat renderer owns no feature API.
export const CommentTransportContext = createContext<CommentTransport>(async () => {
  throw new Error("Conversation transport is unavailable");
});
