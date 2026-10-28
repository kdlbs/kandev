"use client";

import { IconRefresh } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  useWebPush,
  type WebPushStatus,
  type WebPushTestResult,
} from "@/hooks/domains/settings/use-web-push";
import { SettingsGroup } from "./settings-group";

/** Catalog keys per state, resolved at render so they follow the active locale. */
const STATUS_MESSAGE_KEYS: Record<WebPushStatus, string> = {
  loading: "settings:webPushStatusLoading",
  unsupported: "settings:webPushUnsupported",
  denied: "settings:webPushDenied",
  unsubscribed: "settings:webPushStatusUnsubscribed",
  subscribed: "settings:webPushStatusSubscribed",
  "session-expired": "settings:webPushSessionExpired",
  error: "settings:webPushError",
};

const TEST_MESSAGE_KEYS: Record<Exclude<WebPushTestResult, "idle">, string> = {
  sent: "settings:webPushTestSent",
  "no-subscription": "settings:webPushTestNoSubscription",
  failed: "settings:webPushTestFailed",
};

const WARNING_STATUSES: ReadonlySet<WebPushStatus> = new Set([
  "unsupported",
  "denied",
  "session-expired",
  "error",
]);

function PushActions({ push }: { push: ReturnType<typeof useWebPush> }) {
  const { t } = useTranslation();
  const { status, busy } = push;
  const subscribed = status === "subscribed";
  const canToggle = status !== "loading" && status !== "unsupported" && status !== "denied";
  return (
    <div className="flex items-center gap-2">
      <Button
        size="sm"
        variant={subscribed ? "outline" : "default"}
        className="cursor-pointer"
        disabled={busy || !canToggle}
        onClick={() => void (subscribed ? push.disable() : push.enable())}
        data-testid="web-push-toggle"
      >
        {subscribed ? t("settings:webPushDisable") : t("settings:webPushEnable")}
      </Button>
      {subscribed && (
        <Button
          size="sm"
          variant="outline"
          className="cursor-pointer"
          disabled={busy}
          onClick={() => void push.sendTest()}
          data-testid="web-push-test"
        >
          {t("settings:webPushTest")}
        </Button>
      )}
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="cursor-pointer"
              aria-label={t("settings:webPushRefresh")}
              disabled={busy}
              onClick={() => void push.refresh()}
            >
              <IconRefresh className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("settings:webPushRefresh")}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    </div>
  );
}

/**
 * Per-device Web Push control. Subscribing only ever happens from the button
 * here (a user gesture), never on page load. Which events produce a push is
 * chosen in the Notification Events table, where the Web Push provider appears
 * once a device has subscribed.
 */
export function WebPushSection() {
  const { t } = useTranslation();
  const push = useWebPush();
  const warning = WARNING_STATUSES.has(push.status);
  return (
    <SettingsGroup
      title={t("settings:webPushTitle")}
      description={t("settings:webPushDescription")}
      action={<PushActions push={push} />}
    >
      <p
        role="status"
        className={warning ? "text-xs text-amber-600" : "text-xs text-muted-foreground"}
        data-testid="web-push-status"
        data-status={push.status}
      >
        {t(STATUS_MESSAGE_KEYS[push.status])}
      </p>
      {push.testResult !== "idle" && (
        <p role="status" className="text-xs text-muted-foreground">
          {t(TEST_MESSAGE_KEYS[push.testResult])}
        </p>
      )}
    </SettingsGroup>
  );
}
