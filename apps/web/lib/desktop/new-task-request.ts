const listeners = new Set<() => void>();
let pending = false;

export function requestNewTaskCreation(): void {
  if (listeners.size === 0) {
    pending = true;
    return;
  }
  for (const listener of listeners) listener();
}

export function subscribeNewTaskCreationRequests(listener: () => void): () => void {
  listeners.add(listener);
  if (pending) {
    pending = false;
    listener();
  }
  return () => listeners.delete(listener);
}
