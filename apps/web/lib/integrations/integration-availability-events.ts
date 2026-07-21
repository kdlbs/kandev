type Listener = () => void;

const listeners = new Set<Listener>();

export function subscribeIntegrationAvailability(listener: Listener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function invalidateIntegrationAvailability(): void {
  for (const listener of listeners) listener();
}

export async function invalidateIntegrationAvailabilityAfter<T>(request: Promise<T>): Promise<T> {
  const result = await request;
  invalidateIntegrationAvailability();
  return result;
}
