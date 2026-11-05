import type { QueryClient } from "@tanstack/react-query";

const flights = new WeakMap<QueryClient, Map<string, Promise<unknown>>>();

export function submitSandboxOnce<T>(
  client: QueryClient,
  key: string,
  submit: () => Promise<T>,
): Promise<T> {
  let pending = flights.get(client);
  if (!pending) {
    pending = new Map();
    flights.set(client, pending);
  }
  const existing = pending.get(key);
  if (existing) return existing as Promise<T>;
  const result = Promise.resolve()
    .then(submit)
    .finally(() => pending.delete(key));
  pending.set(key, result);
  return result;
}
