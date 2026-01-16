export function linkToTaskSession(taskId: string, sessionId: string): string {
  return `/task/${taskId}/${sessionId}`;
}
