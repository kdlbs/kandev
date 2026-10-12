"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { IconLoader2 } from "@tabler/icons-react";
import { useKubernetesSessions } from "@/hooks/domains/settings/use-kubernetes-settings";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { DesktopSessionTable, MobileSessionList } from "./kubernetes-session-lists";
import { SettingsCard } from "./settings-card";
import { SettingsCardHeader } from "./settings-card-header";
import { settingsActionClassName } from "./settings-control";

type KubernetesSessionsState = ReturnType<typeof useKubernetesSessions>;

export function KubernetesSessionsCard({ state }: { state: KubernetesSessionsState }) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const errorMessage =
    state.error instanceof Error && state.error.message
      ? state.error.message
      : t("executors:kubernetesSessionsFailed");
  return (
    <SettingsCard className="min-w-0 overflow-hidden" data-testid="kubernetes-sessions-card">
      <SettingsCardHeader
        title={t("executors:kubernetesActiveSessions")}
        description={t("executors:kubernetesActiveSessionsDescription")}
        actions={
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void state.refresh().catch(() => undefined)}
            disabled={state.loading}
            className={settingsActionClassName("w-full cursor-pointer md:w-auto")}
          >
            {state.loading ? <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" /> : null}
            {t("executors:refresh")}
          </Button>
        }
      />
      <CardContent className="min-w-0">
        {!state.error && <SessionGuidance />}
        {Boolean(state.error) && (
          <p className="break-words text-sm text-destructive">{errorMessage}</p>
        )}
        {!state.error && !state.loading && state.sessions.length === 0 && (
          <p className="text-sm text-muted-foreground">
            {t("executors:kubernetesNoActiveSessions")}
          </p>
        )}
        {!state.error &&
          state.sessions.length > 0 &&
          (isMobile ? (
            <MobileSessionList sessions={state.sessions} />
          ) : (
            <DesktopSessionTable sessions={state.sessions} />
          ))}
      </CardContent>
    </SettingsCard>
  );
}

function SessionGuidance() {
  const { t } = useTranslation();
  return (
    <p
      className="mb-3 break-words text-xs text-muted-foreground"
      data-testid="kubernetes-session-guidance"
    >
      {t("executors:kubernetesActiveSessionsGuidance")}
    </p>
  );
}
