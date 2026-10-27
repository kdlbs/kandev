import type { TaskSessionState } from "@/lib/types/http";

export function isQuickChatStartupPending(args: {
  startupPending?: boolean;
  sessionState?: TaskSessionState | null;
}): boolean {
  if (args.sessionState) {
    return args.sessionState === "CREATED" || args.sessionState === "STARTING";
  }
  return args.startupPending === true;
}
