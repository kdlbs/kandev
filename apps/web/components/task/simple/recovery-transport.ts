import { createContext } from "react";
export type RecoveryTransport = (
  taskId: string,
  sessionId: string,
  action: "resume" | "fresh_start",
) => Promise<unknown>;
export const RecoveryTransportContext = createContext<RecoveryTransport | null>(null);
