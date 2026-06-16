export type LoadState<T> =
  | { status: "loading" }
  | { status: "ready"; data: T }
  | { status: "error"; message: string };

export function toRouteErrorState(
  error: unknown,
  fallbackMessage = "Failed to load route",
): LoadState<never> {
  return {
    status: "error",
    message: error instanceof Error ? error.message : fallbackMessage,
  };
}
