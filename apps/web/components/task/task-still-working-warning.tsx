import { IconAlertTriangle } from "@tabler/icons-react";

// The §spec:destructive-action-guard "still working" warning, shared by the
// archive and delete confirmation dialogs so both surface identical copy. It is a
// warn-before-proceed banner inside the existing dialog — not a hard block — shown
// only while the task is in-flight (see useTaskInFlight). `count` tailors the
// wording for a bulk selection where at least one task is still working.
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
