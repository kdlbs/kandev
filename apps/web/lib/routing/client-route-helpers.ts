import { t } from "@/lib/i18n";
export type LoadState<T> =
  | { status: "loading" }
  | { status: "ready"; data: T }
  | { status: "error"; message: string };

export function toRouteErrorState(
  error: unknown,
  fallbackMessage = t("common:failedToLoadRoute"),
): LoadState<never> {
  return {
    status: "error",
    message: error instanceof Error ? error.message : fallbackMessage,
  };
}
