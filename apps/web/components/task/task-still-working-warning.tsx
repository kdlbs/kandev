import { IconAlertTriangle } from "@tabler/icons-react";

export function StillWorkingWarning({ count }: { count?: number }) {
  const subject =
    count && count > 1
      ? "One or more of these tasks are still working"
      : "This task is still working";
  return (
    <div
      data-testid="still-working-warning"
      role="alert"
      className="flex items-start gap-2 rounded-md border border-yellow-500/40 bg-yellow-500/10 p-3 text-sm text-yellow-700 dark:text-yellow-300"
    >
      <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-yellow-500" aria-hidden />
      <span>
        {subject} — an agent is generating or running background work. Proceeding now discards work
        that is still in progress.
      </span>
    </div>
  );
}
