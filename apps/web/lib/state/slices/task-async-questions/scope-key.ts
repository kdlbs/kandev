export function taskAsyncQuestionsScopeKey(workspaceId: string, taskId: string) {
  return JSON.stringify([workspaceId, taskId]);
}
