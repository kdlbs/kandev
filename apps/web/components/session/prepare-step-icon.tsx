import { IconAlertTriangle, IconCheck, IconX, IconLoader2 } from "@tabler/icons-react";

export function StepIcon({
  status,
  hasWarning,
  isWarning,
}: {
  status: string;
  hasWarning?: boolean;
  isWarning?: boolean;
}) {
  if (isWarning || (status === "completed" && hasWarning)) {
    return (
      <IconAlertTriangle
        data-testid="prepare-step-warning-icon"
        className="h-3.5 w-3.5 text-amber-500"
      />
    );
  }
  if (status === "completed") {
    return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
  }
  if (status === "failed") {
    return <IconX className="h-3.5 w-3.5 text-destructive" />;
  }
  if (status === "running") {
    return <IconLoader2 className="h-3.5 w-3.5 text-muted-foreground animate-spin" />;
  }
  return <div className="h-3.5 w-3.5 rounded-full border border-muted-foreground/30" />;
}
