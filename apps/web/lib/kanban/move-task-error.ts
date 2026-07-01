export type MoveTaskError = {
  message: string;
  taskId: string;
  sessionId: string | null;
};
