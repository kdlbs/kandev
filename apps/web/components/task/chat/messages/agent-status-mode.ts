import { createContext, useContext } from "react";

/** True inside a chat that shows its own live status line: `AgentStatus` then
 *  drops its RUNNING spinner and idle last-turn duration. */
export const ActivityDisplayContext = createContext(false);

export function useActivityDisplay(): boolean {
  return useContext(ActivityDisplayContext);
}
