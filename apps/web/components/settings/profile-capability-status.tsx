import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { IconAlertCircle, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { ModelConfig } from "@/lib/types/http";
import type { ProfileDiscoveryStatus } from "@/hooks/domains/settings/use-profile-model-capabilities";

const CAPABILITY_STATUS_KEYS: Record<string, string> = {
  probing: "agents:capabilityProbing",
  auth_required: "agents:capabilityAuthRequired",
  not_installed: "agents:capabilityNotInstalled",
  failed: "agents:capabilityProbeFailed",
  loading: "agents:capabilityProbing",
  stale: "agents:profileCapabilitiesStale",
  ready: "agents:profileCapabilitiesReady",
};

function capabilityStatusMessage(
  t: TFunction,
  status: ModelConfig["status"] | ProfileDiscoveryStatus,
): string | null {
  const key = status ? CAPABILITY_STATUS_KEYS[status] : undefined;
  return key ? t(key) : null;
}

export function CapabilityStatusMessage({
  status,
}: {
  status: ModelConfig["status"] | ProfileDiscoveryStatus;
}) {
  const { t } = useTranslation();
  const msg = capabilityStatusMessage(t, status);
  if (!msg) return null;
  return (
    <p
      data-testid="profile-capability-status"
      data-status={status}
      className="text-xs text-muted-foreground"
    >
      {msg}
    </p>
  );
}

export function RefreshCapabilitiesButton({
  onRefresh,
  isLoading,
  error,
}: {
  onRefresh: () => Promise<void>;
  isLoading: boolean;
  error: string | null;
}) {
  const { t } = useTranslation();
  return (
    <div className="w-full sm:w-auto">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            onClick={onRefresh}
            disabled={isLoading}
            className="min-h-9 w-full cursor-pointer px-3 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11 sm:w-auto"
            data-testid="profile-refresh-capabilities"
          >
            <IconRefresh className={`h-4 w-4 ${isLoading ? "animate-spin" : ""}`} />
            <span>{t("agents:refreshCapabilities")}</span>
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          <p>{t("agents:refreshCapabilitiesTooltip")}</p>
        </TooltipContent>
      </Tooltip>
      {error && (
        <Tooltip>
          <TooltipTrigger asChild>
            <div className="flex items-center">
              <IconAlertCircle className="h-4 w-4 text-amber-500" />
            </div>
          </TooltipTrigger>
          <TooltipContent>
            <p className="max-w-xs">{t("agents:failedToRefresh", { error })}</p>
          </TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}
