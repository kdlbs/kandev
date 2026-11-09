export function shouldNavigateAfterTaskCreate(
  withAgent: boolean,
  isPassthroughProfile: boolean,
  planMode: boolean | undefined,
  sessionId: string | null,
) {
  return (withAgent && isPassthroughProfile) || !!(planMode && sessionId);
}

export type FinishTaskCreateNavigationArgs = {
  planMode: boolean | undefined;
  newSessionId: string | null;
  autoFocusNewTasks: boolean;
  withAgent: boolean;
  isPassthroughProfile: boolean;
  activatePlan: (sessionId: string) => void;
  openPassthroughTask: () => void;
};

export function finishTaskCreateNavigation(args: FinishTaskCreateNavigationArgs) {
  if (args.planMode && args.newSessionId) {
    args.activatePlan(args.newSessionId);
  } else if (args.autoFocusNewTasks && args.withAgent && args.isPassthroughProfile) {
    args.openPassthroughTask();
  }
}
