import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { CapabilityStatus } from "@/lib/types/http";

export function ModelConfigResolutionStatus({
  status,
  error,
  isLoading,
  onRetry,
}: {
  status: CapabilityStatus | undefined;
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

  if (status !== "failed" && status !== "auth_required" && !error) {
    return null;
  }

  return (
    <div
      className="flex flex-wrap items-center gap-2 text-xs text-destructive"
      data-testid="model-config-resolution-error"
    >
      <span>
        {status === "auth_required"
          ? t("agents:modelOptionsAuthenticationRequired")
          : t("agents:modelOptionsResolutionFailed")}
      </span>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="min-h-10 px-2"
        onClick={() => void onRetry()}
      >
        {t("agents:retryModelOptions")}
      </Button>
    </div>
  );
}
