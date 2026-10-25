import { useSessionPendingInput, type PendingInput } from "@/hooks/use-task-pending-input";
import { t } from "@/lib/i18n";
import { formatTaskSessionStateLabel } from "@/lib/ui/state-labels";
import { getSessionStateIcon } from "@/lib/ui/state-icons";
import type { ForegroundActivity, TaskSessionState } from "@/lib/types/http";

// A catalog key keeps the label responsive to the active locale.
const BACKGROUND_RUNNING_LABEL_KEY = "task:workingInBackground";

function sessionStateLabel(
  state: TaskSessionState,
  foregroundActivity: ForegroundActivity | null,
  pending: PendingInput,
  parkedOnBackgroundWork: boolean,
): string {
  const canRequestInput = state === "RUNNING" || state === "WAITING_FOR_INPUT";
  if (canRequestInput && pending.permission) return t("task:permissionRequested");
  if (canRequestInput && pending.clarification) {
    return formatTaskSessionStateLabel("WAITING_FOR_INPUT");
  }
  if (canRequestInput && foregroundActivity === "background") {
    return t(BACKGROUND_RUNNING_LABEL_KEY);
  }
  if (canRequestInput && parkedOnBackgroundWork) {
    return t(BACKGROUND_RUNNING_LABEL_KEY);
  }
  return formatTaskSessionStateLabel(state);
}

export function MobileSessionStateBadge({
  sessionId,
  state,
  foregroundActivity,
  parkedOnBackgroundWork,
  testId,
}: {
  sessionId: string;
  state: TaskSessionState | null;
  foregroundActivity: ForegroundActivity | null;
  parkedOnBackgroundWork: boolean;
  testId?: string;
}) {
  const pending = useSessionPendingInput(sessionId);
  if (!state) return null;
  const label = sessionStateLabel(state, foregroundActivity, pending, parkedOnBackgroundWork);
  return (
    <span
      data-testid={testId}
      title={label}
      className="flex items-center gap-1 whitespace-nowrap text-[10px] font-medium leading-none text-muted-foreground shrink-0"
    >
      {getSessionStateIcon(state, "h-3 w-3 shrink-0", {
        foregroundActivity,
        hasPendingClarification: pending.clarification,
        hasPendingPermission: pending.permission,
        parkedOnBackgroundWork,
      })}
      {label}
    </span>
  );
}
