export function isQuickChatStartupPending(args: {
  startupPending?: boolean;
  sessionState?: string | null;
}): boolean {
  if (args.sessionState) {
    return args.sessionState === "CREATED" || args.sessionState === "STARTING";
  }
  return args.startupPending === true;
}
