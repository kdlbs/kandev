import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { CapabilityStatus } from "@/lib/types/http";
import { settingsActionClassName } from "./settings-control";

export function ModelConfigResolutionStatus({
  status,
  reasonCode,
  error,
  isLoading,
  onRetry,
}: {
  status: CapabilityStatus | undefined;
  reasonCode?: string;
  error: string | null;
  isLoading: boolean;
  onRetry: () => Promise<void>;
}) {
  const { t } = useTranslation();
  if (isLoading || status === "probing") {
    return (
      <p className="text-xs text-muted-foreground" data-testid="model-config-resolution-loading">
        {t("agents:resolvingModelOptions")}
      </p>
    );
  }

  const hasPolicyRecovery = status === "unsupported" && reasonCode === "policy_unverified";
  if (status !== "failed" && status !== "auth_required" && !error && !hasPolicyRecovery) {
    return null;
  }

  let message = t("agents:modelOptionsResolutionFailed");
  if (status === "auth_required") {
    message = t("agents:modelOptionsAuthenticationRequired");
  } else if (hasPolicyRecovery) {
    message = t("agents:copilotPluginModelOptionsRecovery");
  }

  return (
    <div
      className="flex min-w-0 items-start gap-2 text-xs text-destructive"
      aria-live="polite"
      data-testid="model-config-resolution-error"
    >
      <span className="min-w-0 flex-1">{message}</span>
      <Button
        type="button"
        variant="ghost"
        className={settingsActionClassName("shrink-0 px-2")}
        onClick={() => void onRetry()}
      >
        {t("agents:retryModelOptions")}
      </Button>
    </div>
  );
}
