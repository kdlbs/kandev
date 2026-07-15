const NEW_TASK_REQUEST_EVENT = "kandev-web-request-new-task";

export function requestNewTaskCreation(): void {
  window.dispatchEvent(new Event(NEW_TASK_REQUEST_EVENT));
}

export function subscribeNewTaskCreationRequests(listener: () => void): () => void {
  window.addEventListener(NEW_TASK_REQUEST_EVENT, listener);
  return () => window.removeEventListener(NEW_TASK_REQUEST_EVENT, listener);
}
