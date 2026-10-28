"use client";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { IconPlayerPlay } from "@tabler/icons-react";
import type { SSHRoute } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { SettingsGroup, SettingsRow } from "../settings-group";
import { settingsActionClassName } from "../settings-control";

export function ConnectionOverview({
  route,
  manager,
  onAccess,
}: {
  route: SSHRoute;
  manager: SSHManager;
  onAccess: () => void;
}) {
  const { t } = useTranslation();
  const c = route.destination;
  const identity = manager.identities.find((item) => item.id === c.identity_id);
  const value = (text: string) => <span className="text-sm break-all text-right">{text}</span>;
  return (
    <div className="space-y-8" data-testid="ssh-connection-overview">
      <SettingsGroup title={t("settings:sshManager.ui.connectionDetails")}>
        <SettingsRow
          className="max-md:gap-1"
          controlWrapperClassName="max-md:min-h-0 [@media(pointer:coarse)]:min-h-0"
          label={t("settings:sshManager.ui.endpoint")}
          control={value(`${c.user}@${c.host}:${c.port}`)}
        />
      </SettingsGroup>
      <SettingsGroup title={t("settings:sshManager.ui.identityDetails")}>
        <SettingsRow
          className="max-md:gap-1"
          controlWrapperClassName="max-md:min-h-0 [@media(pointer:coarse)]:min-h-0"
          label={t("settings:sshManager.identity")}
          control={value(identity?.label ?? t("settings:sshManager.ui.states.unavailable"))}
        />
        <SettingsRow
          className="max-md:gap-1"
          controlWrapperClassName="max-md:min-h-0 [@media(pointer:coarse)]:min-h-0"
          label={t("settings:sshManager.bastion")}
          control={value(route.bastion?.name ?? t("settings:sshManager.direct"))}
        />
      </SettingsGroup>
      <div className="space-y-3">
        <Button
          className={settingsActionClassName()}
          disabled={manager.busy}
          onClick={() => {
            if (!c.fingerprint) onAccess();
            else void manager.test(route).catch(() => undefined);
          }}
        >
          <IconPlayerPlay className="size-3.5 mr-1.5" aria-hidden />
          {t(c.fingerprint ? "settings:sshManager.test" : "settings:sshManager.ui.setupAccess")}
        </Button>
        <p className="text-xs text-muted-foreground">
          {t("settings:sshManager.executorReadiness")}
        </p>
      </div>
    </div>
  );
}
