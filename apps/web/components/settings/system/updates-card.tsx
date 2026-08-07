"use client";

import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@kandev/ui/alert-dialog";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { IconDownload, IconExternalLink, IconRefresh } from "@tabler/icons-react";
import { useSelfUpdate, type SelfUpdateController } from "@/hooks/domains/system/use-self-update";
import {
  useDesktopUpdater,
  type DesktopUpdaterController,
} from "@/hooks/domains/system/use-desktop-updater";
import { useUpdates } from "@/hooks/domains/system/use-updates";
import { formatDateTime } from "@/lib/i18n/formats";
import type { UpdatesResponse } from "@/lib/types/system";
import { SettingsCard } from "../settings-card";
import { SelfUpdateProgress } from "./self-update-progress";
import { UpdateChannelControl, useUpdateChannelDraft } from "./update-channel-control";

interface ApplyGate {
  canApply: boolean;
  cannotApplyReason?: string;
  manualCommands: string[];
}

type UpdatesCardProps = {
  reloadDocument?: () => void;
};

function reloadCurrentDocument(): void {
  window.location.reload();
}

function formatChecked(value: string | number | null | undefined, t: TFunction): string {
  if (!value) return t("system:updatesNever");
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return String(value);
  return formatDateTime(d);
}

function retryAfterSeconds(message: string): number | null {
  const match = /retry.*?(\d+)/i.exec(message);
  return match ? Number(match[1]) : null;
}

function getApplyGate(updates: UpdatesResponse | null | undefined): ApplyGate {
  const install = updates?.install;
  const available = updates?.update_available === true;
  const canApply =
    available &&
    install?.running_as_service === true &&
    install.managed_service === true &&
    updates?.apply_supported === true;
  return {
    canApply,
    cannotApplyReason: updates?.apply_unsupported_reason,
    manualCommands: updates?.manual_commands ?? [],
  };
}

function serviceCardView(
  updates: UpdatesResponse | null | undefined,
  selfUpdate: SelfUpdateController,
) {
  const available = updates?.update_available === true;
  const gate = getApplyGate(updates);
  return {
    current: updates?.current ?? "-",
    latest: updates?.latest ?? "-",
    available,
    showApply: gate.canApply && !selfUpdate.isUpdating && selfUpdate.phase !== "done",
    showManual: available && !gate.canApply && !selfUpdate.isUpdating,
    cannotApplyReason: gate.cannotApplyReason,
    manualCommands: gate.manualCommands,
  };
}

function ChannelPendingNotice({ pending, saving }: { pending: boolean; saving: boolean }) {
  const { t } = useTranslation();
  if (!pending) return null;
  return (
    <p className="text-xs text-muted-foreground" data-testid="system-updates-channel-pending">
      {saving ? t("settings:updateChannelSavingNotice") : t("settings:updateChannelUnsavedNotice")}
    </p>
  );
}

export function UpdatesCard({ reloadDocument = reloadCurrentDocument }: UpdatesCardProps = {}) {
  const desktopUpdater = useDesktopUpdater();
  if (desktopUpdater.available) {
    return <DesktopUpdatesCard updater={desktopUpdater} />;
  }
  return <ServiceUpdatesCard reloadDocument={reloadDocument} />;
}

function ServiceUpdatesCard({ reloadDocument }: { reloadDocument: () => void }) {
  const { t } = useTranslation();
  const { updates, check, saveChannel, isChecking, error } = useUpdates();
  const selfUpdate = useSelfUpdate({ latestVersion: updates?.latest, onComplete: reloadDocument });
  const channel = useUpdateChannelDraft(updates, saveChannel);
  const view = serviceCardView(updates, selfUpdate);
  const channelPending = channel.isDirty || channel.isSaving;

  return (
    <SettingsCard isDirty={channelPending} data-testid="system-updates-card">
      <CardHeader>
        <UpdatesHeader available={view.available && !channelPending} />
      </CardHeader>
      <CardContent className="space-y-4">
        <UpdateChannelControl {...channel} />
        <ChannelPendingNotice pending={channelPending} saving={channel.isSaving} />
        <VersionGrid
          current={view.current}
          latest={channelPending ? "-" : view.latest}
          latestLabel={
            channel.draft === "nightly"
              ? t("settings:updateChannelLatestNightly")
              : t("settings:updateChannelLatestRelease")
          }
        />
        <LastChecked checkedAt={channelPending ? undefined : updates?.latest_checked_at} />
        <UpdateActions
          checking={isChecking}
          disabled={channelPending}
          showApply={view.showApply && !channelPending && !isChecking}
          latest={view.latest}
          url={channelPending ? undefined : updates?.latest_url}
          onCheck={() => ignoreFailure(check())}
          onApply={selfUpdate.start}
        />
        <ManualUpdateInstructions
          show={view.showManual && !channelPending}
          reason={view.cannotApplyReason}
          commands={view.manualCommands}
        />
        <SelfUpdateProgress
          phase={selfUpdate.phase}
          targetVersion={selfUpdate.targetVersion}
          errorMessage={selfUpdate.errorMessage}
          onDismiss={selfUpdate.dismiss}
        />
        <UpdateError error={error} retryAfter={error ? retryAfterSeconds(error) : null} />
      </CardContent>
    </SettingsCard>
  );
}

function DesktopUpdatesCard({ updater }: { updater: DesktopUpdaterController }) {
  const view = desktopCardView(updater);

  return (
    <Card data-testid="system-updates-card">
      <CardHeader>
        <UpdatesHeader available={view.available} />
      </CardHeader>
      <CardContent className="space-y-4">
        <VersionGrid current={view.current} latest={view.latest} />
        <LastChecked checkedAt={updater.state?.checkedAtEpochMs} />
        <UpdateActions
          checking={view.checking}
          showApply={view.showApply}
          latest={view.latest}
          url={updater.state?.releaseUrl ?? undefined}
          onCheck={() => ignoreFailure(updater.check())}
          onApply={() => ignoreFailure(updater.install())}
          desktop
        />
        <ManualUpdateInstructions
          show={view.available && !view.installSupported}
          reason={updater.state?.installUnsupportedReason ?? undefined}
          commands={[]}
        />
        <DesktopCurrentStatus phase={updater.state?.phase} />
        <DesktopUpdateProgress updater={updater} />
        <UpdateError error={updater.error} retryAfter={null} />
      </CardContent>
    </Card>
  );
}

function desktopCardView(updater: DesktopUpdaterController) {
  const state = updater.state;
  if (!state) {
    return {
      available: false,
      checking: updater.checking,
      showApply: false,
      installSupported: false,
      current: "-",
      latest: "-",
    };
  }
  const available = state.phase === "available";
  const installing = updater.installing || ["downloading", "installing"].includes(state.phase);
  const busy = updater.checking || installing || state.phase === "checking";
  return {
    available,
    checking: busy,
    showApply: available && state.installSupported === true && !busy,
    installSupported: state.installSupported === true,
    current: state.currentVersion,
    latest: state.latestVersion ?? (state.phase === "up-to-date" ? state.currentVersion : "-"),
  };
}

function DesktopCurrentStatus({ phase }: { phase: string | undefined }) {
  const { t } = useTranslation();
  if (phase !== "up-to-date") return null;
  return (
    <p className="text-xs text-muted-foreground" data-testid="system-updates-current-status">
      {t("system:updatesUpToDate")}
    </p>
  );
}

async function ignoreFailure(operation: Promise<unknown>): Promise<void> {
  await operation.catch(() => undefined);
}

function DesktopUpdateProgress({ updater }: { updater: DesktopUpdaterController }) {
  const { t } = useTranslation();
  const state = updater.state;
  if (state?.phase !== "downloading" && state?.phase !== "installing") return null;
  // Built outside JSX, which is why `mode: "jsx-only"` never reported it.
  let detail = t("system:updatesInstalling");
  if (state.phase === "downloading") {
    const downloaded = state.downloadedBytes ?? 0;
    detail = state.totalBytes
      ? t("system:updatesDownloadingOfTotal", {
          downloaded,
          total: state.totalBytes,
        })
      : t("system:updatesDownloading", { downloaded });
  }
  return (
    <div
      className="flex items-center gap-2 text-xs text-muted-foreground"
      data-testid="system-updates-progress"
      data-phase={state.phase}
    >
      <Spinner className="size-3.5" />
      {detail}
    </div>
  );
}

function UpdatesHeader({ available }: { available: boolean }) {
  const { t } = useTranslation();
  return (
    <CardTitle className="text-base flex items-center gap-2">
      <IconRefresh className="h-4 w-4" />
      {t("system:updatesTitle")}
      {available && (
        <Badge variant="default" className="text-[10px]" data-testid="system-updates-badge">
          {t("system:updateAvailable")}
        </Badge>
      )}
    </CardTitle>
  );
}

function VersionGrid({
  current,
  latest,
  latestLabel,
}: {
  current: string;
  latest: string;
  latestLabel?: string;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="grid min-w-0 grid-cols-1 gap-3 text-sm sm:grid-cols-2"
      data-testid="system-updates-versions"
    >
      <VersionValue
        label={t("system:updatesCurrentVersion")}
        value={current}
        testId="system-updates-current"
      />
      <VersionValue
        label={latestLabel ?? t("system:updatesLatestRelease")}
        value={latest}
        testId="system-updates-latest"
      />
    </div>
  );
}

function VersionValue({ label, value, testId }: { label: string; value: string; testId: string }) {
  return (
    <div className="min-w-0">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="break-all font-mono text-sm" data-testid={testId}>
        {value}
      </div>
    </div>
  );
}

function LastChecked({ checkedAt }: { checkedAt?: string | number | null }) {
  const { t } = useTranslation();
  return (
    <div className="text-xs text-muted-foreground" data-testid="system-updates-checked-at">
      {t("system:updatesLastChecked", { at: formatChecked(checkedAt, t) })}
    </div>
  );
}

interface UpdateActionsProps {
  checking: boolean;
  disabled?: boolean;
  showApply: boolean;
  latest: string;
  url?: string;
  onCheck: () => Promise<void>;
  onApply: () => Promise<void>;
  desktop?: boolean;
}

function UpdateActions(props: UpdateActionsProps) {
  return (
    <div
      className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center"
      data-testid="system-updates-actions"
    >
      <CheckNowButton checking={props.checking} disabled={props.disabled} onCheck={props.onCheck} />
      <ReleaseNotesLink url={props.url} />
      <ApplyUpdateDialog
        showApply={props.showApply}
        latest={props.latest}
        onApply={props.onApply}
        desktop={props.desktop}
      />
    </div>
  );
}

function CheckNowButton({
  checking,
  disabled,
  onCheck,
}: {
  checking: boolean;
  disabled?: boolean;
  onCheck: () => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={checking || disabled}
      onClick={() => void onCheck()}
      className="cursor-pointer"
      data-testid="system-updates-check"
    >
      {checking ? (
        <Spinner className="size-3.5 mr-1" />
      ) : (
        <IconRefresh className="h-3.5 w-3.5 mr-1" />
      )}
      {t("system:updatesCheckNow")}
    </Button>
  );
}

function ReleaseNotesLink({ url }: { url?: string }) {
  const { t } = useTranslation();
  if (!url) return null;
  return (
    <Button
      asChild
      variant="ghost"
      size="sm"
      className="cursor-pointer"
      data-testid="system-updates-release-link"
    >
      <a href={url} target="_blank" rel="noreferrer">
        {t("system:updatesReleaseNotes")}
        <IconExternalLink className="h-3.5 w-3.5 ml-1" />
      </a>
    </Button>
  );
}

interface ApplyUpdateDialogProps {
  showApply: boolean;
  latest: string;
  onApply: () => Promise<void>;
  desktop?: boolean;
}

function ApplyUpdateDialog({ showApply, latest, onApply, desktop }: ApplyUpdateDialogProps) {
  const { t } = useTranslation();
  if (!showApply) return null;
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button size="sm" className="cursor-pointer" data-testid="system-updates-apply">
          <IconDownload className="h-3.5 w-3.5 mr-1" />
          {t("system:updatesApply")}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("system:updatesApplyConfirmTitle")}</AlertDialogTitle>
          <AlertDialogDescription className="text-left">
            {/* The version is a value. */}
            {desktop
              ? t("system:updatesApplyConfirmDesktop", { version: latest })
              : t("system:updatesApplyConfirmService", { version: latest })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
          <AlertDialogAction
            className="cursor-pointer"
            onClick={() => void onApply()}
            data-testid="system-updates-apply-confirm"
          >
            {t("system:updatesApply")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function ManualUpdateInstructions({
  show,
  reason,
  commands,
}: {
  show: boolean;
  reason?: string;
  commands: string[];
}) {
  if (!show || !reason) return null;
  return (
    <div className="space-y-2 text-xs text-muted-foreground" data-testid="system-updates-manual">
      {/* `reason` and every manual command come from the updates API. */}
      <p>{reason}</p>
      <ManualCommands commands={commands} />
    </div>
  );
}

function ManualCommands({ commands }: { commands: string[] }) {
  if (commands.length === 0) return null;
  return (
    <div className="space-y-1">
      {commands.map((cmd) => (
        <code key={cmd} className="block break-all rounded bg-muted px-2 py-1 font-mono">
          {cmd}
        </code>
      ))}
    </div>
  );
}

function UpdateError({ error, retryAfter }: { error: string | null; retryAfter: number | null }) {
  const { t } = useTranslation();
  if (!error) return null;
  return (
    <p className="text-xs text-destructive" data-testid="system-updates-error">
      {retryAfter ? t("system:updatesRetryAfter", { count: retryAfter }) : error}
    </p>
  );
}
