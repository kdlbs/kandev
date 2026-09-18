import { createContext } from "react";
import type { ClarificationAnswer, Message } from "@/lib/types/http";
export type ClarificationTransport = {
  respond: (
    pendingId: string,
    body: { answers?: ClarificationAnswer[]; rejected: boolean; reject_reason?: string },
  ) => Promise<{
    state: "ok" | "error" | "expired";
    claimed?: boolean;
    status?: "answered" | "rejected";
    answers?: ClarificationAnswer[];
  }>;
  updateMessage: (message: Message) => void;
};
// Native task chat keeps its canonical transport. Feature shells may supply a
// narrower resolver and disable optimistic writes for projected presentations.
export const ClarificationTransportContext = createContext<ClarificationTransport | undefined>(
  undefined,
);
