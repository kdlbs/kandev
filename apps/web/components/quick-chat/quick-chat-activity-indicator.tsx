import { cn } from "@/lib/utils";
import type { QuickChatActivityState } from "@/lib/state/slices/ui/quick-chat-activity-selectors";

type QuickChatActivityIndicatorProps = {
  activity: QuickChatActivityState;
  className?: string;
};

/** Decorative activity bubble shared by every Quick Chat entry point. */
export function QuickChatActivityIndicator({
  activity,
  className,
}: QuickChatActivityIndicatorProps) {
  if (!activity) return null;

  return (
    <span
      aria-hidden="true"
      className={cn(
        "absolute -right-1 -top-1 h-2 w-2 rounded-full ring-2 ring-background",
        activity === "running" ? "bg-blue-500" : "bg-emerald-500",
        className,
      )}
      data-legacy-testid="quick-chat-unseen-dot"
      data-state={activity}
      data-testid="quick-chat-activity-indicator"
    />
  );
}
