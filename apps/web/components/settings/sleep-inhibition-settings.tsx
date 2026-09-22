"use client";

import { useEffect, useRef, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Spinner } from "@kandev/ui/spinner";
import { Switch } from "@kandev/ui/switch";
import { SettingsInfo } from "./settings-info";
import { IconAlertCircle } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { SettingsCard } from "./settings-card";
import { SettingsRow } from "./settings-group";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { useSleepInhibitionSettings } from "@/hooks/domains/settings/use-sleep-inhibition-settings";
import type { SleepInhibitionResponse } from "@/lib/types/system";
import { settingsActionClassName } from "./settings-control";

const MACOS_COMMAND = "/usr/bin/caffeinate -i -w <kandev-pid>";
// i18n-exempt: Win32 API signature shown verbatim as code.
const WINDOWS_API = "SetThreadExecutionState(ES_CONTINUOUS | ES_SYSTEM_REQUIRED)";
// i18n-exempt: D-Bus method call shown verbatim as code; its arguments are
// wire values the systemd API matches, not display copy.
const LINUX_METHOD =
  'org.freedesktop.login1.Manager.Inhibit("sleep", "Kandev", "A Kandev task is running", "block")';
const CODE_CLASS = "break-all rounded bg-muted px-1 py-0.5 font-mono text-[0.7rem]";

function statusMessageKey(response: SleepInhibitionResponse): string {
  if (response.status.issue === "unsupported_platform")
    return "settings:sleepInhibitionUnsupported";
  if (response.status.issue === "system_service_unavailable") {
    return "settings:sleepInhibitionServiceUnavailable";
  }
  if (response.status.issue === "request_failed") return "settings:sleepInhibitionRequestFailed";
  if (response.status.active) return "settings:sleepInhibitionActive";
  return "settings:sleepInhibitionAvailable";
}

type SleepInhibitionState = {
  snapshot: SleepInhibitionResponse | null;
  draft: boolean | null;
  setDraft: (value: boolean) => void;
  loading: boolean;
  loadFailed: boolean;
  saveFailed: boolean;
  isDirty: boolean;
  isAdmin: boolean;
  canEdit: boolean;
  saved: boolean | undefined;
  reload: () => Promise<void>;
};

function useSleepInhibitionState(): SleepInhibitionState {
  const { t } = useTranslation();
  const role = useAppStore((state) => state.auth.user?.role);
  const remote = useSleepInhibitionSettings();
  const snapshot = remote.response;
  const [draft, setDraftState] = useState<boolean | null>(null);
  const [saveFailed, setSaveFailed] = useState(false);
  const isAdmin = role === undefined || role === "admin";
  const saved = snapshot?.settings.enabled;
  const lastSaved = useRef<boolean | undefined>(undefined);

  useEffect(() => {
    if (saved === undefined) return;
    setDraftState((current) =>
      current === null || current === lastSaved.current ? saved : current,
    );
    lastSaved.current = saved;
  }, [saved]);

  const draftValue = draft ?? saved ?? false;
  const isDirty = saved !== undefined && draft !== null && draft !== saved;
  const loading = remote.loading || !remote.loaded;
  const canEdit = isAdmin && !loading && snapshot !== null;

  useSettingsSaveContributor({
    id: "general-task-sleep-inhibition",
    order: 30,
    revision: draftValue ? "enabled" : "disabled",
    isDirty,
    canSave: canEdit,
    invalidReason: !isAdmin ? t("settings:sleepInhibitionAdminOnly") : undefined,
    save: async () => {
      if (!canEdit) throw new Error(t("settings:sleepInhibitionAdminOnly"));
      const submitted = draft ?? saved ?? false;
      setSaveFailed(false);
      try {
        const response = await remote.save({ enabled: submitted });
        setDraftState((current) => (current === submitted ? response.settings.enabled : current));
      } catch (error) {
        setSaveFailed(true);
        throw error;
      }
    },
    discard: () => {
      if (saved !== undefined) setDraftState(saved);
      setSaveFailed(false);
    },
  });

  return {
    snapshot,
    draft,
    setDraft: setDraftState,
    loading,
    loadFailed: remote.error,
    saveFailed,
    isDirty,
    isAdmin,
    canEdit,
    saved,
    reload: remote.refresh,
  };
}

function SleepInhibitionLoadError({
  onRetry,
  withinGroup = false,
}: {
  onRetry: () => void;
  withinGroup?: boolean;
}) {
  const { t } = useTranslation();
  const content = (
    <div className="py-6">
      <Alert variant="destructive">
        <IconAlertCircle className="size-4" />
        <AlertDescription>{t("settings:sleepInhibitionLoadFailed")}</AlertDescription>
      </Alert>
      <Button variant="outline" className={settingsActionClassName("mt-3")} onClick={onRetry}>
        {t("settings:sleepInhibitionRetry")}
      </Button>
    </div>
  );
  if (withinGroup) return <div data-testid="sleep-inhibition-settings">{content}</div>;
  return (
    <SettingsCard data-testid="sleep-inhibition-settings">
      <CardContent>{content}</CardContent>
    </SettingsCard>
  );
}

function SleepInhibitionInfoDetails() {
  const { t } = useTranslation();
  return (
    <>
      <ul className="list-disc space-y-1 pl-4">
        <li>
          <Trans
            i18nKey="settings:sleepInhibitionInfoMacos"
            values={{ command: MACOS_COMMAND, idleOption: "-i", waitOption: "-w" }}
          >
            <code className={CODE_CLASS} />
            <code className={CODE_CLASS} />
            <code className={CODE_CLASS} />
          </Trans>
        </li>
        <li>
          <Trans i18nKey="settings:sleepInhibitionInfoWindows" values={{ api: WINDOWS_API }}>
            <code className={CODE_CLASS} />
          </Trans>
        </li>
        <li>
          <Trans i18nKey="settings:sleepInhibitionInfoLinux" values={{ method: LINUX_METHOD }}>
            <code className={CODE_CLASS} />
          </Trans>
        </li>
      </ul>
      <p>{t("settings:sleepInhibitionInfoRelease")}</p>
    </>
  );
}

function SleepInhibitionInfoTooltipContent() {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <p className="font-medium">{t("settings:sleepInhibitionInfoTitle")}</p>
      <p>{t("settings:sleepInhibitionInfoTrigger")}</p>
      <SleepInhibitionInfoDetails />
    </div>
  );
}

function SleepInhibitionInfoTooltip() {
  const { t } = useTranslation();
  return (
    <SettingsInfo label={t("settings:sleepInhibitionSwitchLabel")} testId="sleep-inhibition-info">
      <SleepInhibitionInfoTooltipContent />
      <p>{t("settings:sleepInhibitionDescription")}</p>
      <p>{t("settings:sleepInhibitionSwitchHint")}</p>
      <p>{t("settings:sleepInhibitionCaveat")}</p>
    </SettingsInfo>
  );
}

function SleepInhibitionCard({
  state,
  withinGroup = false,
}: {
  state: SleepInhibitionState;
  withinGroup?: boolean;
}) {
  const { t } = useTranslation();
  const snapshot = state.snapshot;
  if (!snapshot) return null;
  const control = (
    <Switch
      id="task-sleep-inhibition"
      checked={state.draft ?? snapshot.settings.enabled}
      disabled={!state.canEdit}
      data-testid="sleep-inhibition-switch"
      data-settings-dirty={state.isDirty}
      onCheckedChange={state.setDraft}
      className="shrink-0 cursor-pointer"
    />
  );
  const row = (
    <SettingsRow
      data-testid="sleep-inhibition-control-row"
      label={t("settings:sleepInhibitionSwitchLabel")}
      description={t("settings:sleepShort")}
      info={<SleepInhibitionInfoTooltip />}
      controlId="task-sleep-inhibition"
      touchTarget="switch"
      isDirty={state.isDirty}
      control={control}
    />
  );
  const body = (
    <>
      {withinGroup ? (
        row
      ) : (
        <div
          className="flex min-h-11 items-center justify-between gap-4"
          data-testid="sleep-inhibition-control-row"
        >
          <div className="min-w-0 space-y-0.5">
            <Label htmlFor="task-sleep-inhibition">
              {t("settings:sleepInhibitionSwitchLabel")}
            </Label>
            <p className="text-xs text-muted-foreground">
              {t("settings:sleepInhibitionSwitchHint")}
            </p>
          </div>
          {control}
        </div>
      )}

      <div className="rounded-md border border-border/70 bg-muted/20 p-3 text-sm">
        <p data-testid="sleep-inhibition-status">
          {t("settings:sleepInhibitionStatusLabel")}: {t(statusMessageKey(snapshot))}
        </p>
      </div>

      {!state.isAdmin && (
        <p className="text-sm text-muted-foreground">{t("settings:sleepInhibitionAdminOnly")}</p>
      )}
      {state.saveFailed && (
        <Alert variant="destructive">
          <IconAlertCircle className="size-4" />
          <AlertDescription>{t("settings:sleepInhibitionSaveFailed")}</AlertDescription>
        </Alert>
      )}
    </>
  );
  if (withinGroup) {
    return (
      <div className="min-w-0 space-y-4 py-3" data-testid="sleep-inhibition-settings">
        {body}
      </div>
    );
  }
  return (
    <SettingsCard
      isDirty={state.isDirty}
      className="min-w-0 w-full"
      data-testid="sleep-inhibition-settings"
    >
      <CardHeader>
        <CardTitle className="relative flex items-center gap-1 pr-11 text-base sm:pr-0">
          <span className="min-w-0">{t("settings:sleepInhibitionTitle")}</span>
          <SleepInhibitionInfoTooltip />
        </CardTitle>
        <CardDescription>{t("settings:sleepInhibitionDescription")}</CardDescription>
      </CardHeader>
      <CardContent className="min-w-0 space-y-4">{body}</CardContent>
    </SettingsCard>
  );
}

export function SleepInhibitionSettings({
  withinGroup = false,
  onAttentionChange,
}: {
  withinGroup?: boolean;
  onAttentionChange?: (needsAttention: boolean) => void;
}) {
  const { t } = useTranslation();
  const state = useSleepInhibitionState();
  useEffect(() => {
    onAttentionChange?.(state.loadFailed || state.saveFailed);
  }, [onAttentionChange, state.loadFailed, state.saveFailed]);

  if (state.loading && !state.snapshot) {
    const loadingContent = (
      <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
        <Spinner className="size-4" />
        {t("settings:sleepInhibitionLoading")}
      </div>
    );
    if (withinGroup) return <div data-testid="sleep-inhibition-settings">{loadingContent}</div>;
    return (
      <SettingsCard data-testid="sleep-inhibition-settings">
        <CardContent>{loadingContent}</CardContent>
      </SettingsCard>
    );
  }
  if (state.loadFailed && !state.snapshot) {
    return (
      <SleepInhibitionLoadError onRetry={() => void state.reload()} withinGroup={withinGroup} />
    );
  }
  return <SleepInhibitionCard state={state} withinGroup={withinGroup} />;
}
