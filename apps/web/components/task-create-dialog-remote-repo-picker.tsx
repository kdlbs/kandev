"use client";

import { useMemo, useState } from "react";
import { IconLink } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type {
  RemoteRepository,
  UseRemoteRepositoriesResult,
} from "@/hooks/domains/integrations/use-remote-repositories";
import { parseGitHubAnyUrl } from "@/hooks/domains/github/use-pr-info-by-url";
import type { TaskRemoteRepoRow } from "@/components/task-create-dialog-types";
import { useTaskCreateDialogPopoverContainer } from "@/hooks/use-task-create-dialog-popover-container";
import { RemoteRepositoryProviderIcon } from "@/components/task-create-dialog-remote-repo-provider-tabs";
import {
  RemoteRepositoryPickerPanel,
  type RemoteRepositoryTextSubmitResult,
} from "@/components/remote-repository-picker";
import { remoteRepositoryMatchesSelection } from "./task-create-dialog-remote-repo-identity";
import {
  looksLikeURL,
  looksLikeSupportedRemoteURL,
} from "@/components/workspace-source-picker/remote-url";
import { Trans, useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
import type { RemoteRepoChipProps } from "./task-create-dialog-remote-repo-chip";

const TRUNCATE_THRESHOLD = 30;

// --- Repo pill ---------------------------------------------------------------

export function RemoteRepoPill({
  row,
  accessibleRepos,
  selectedRepositoryIdentities,
  onURLChange,
  disabled,
}: {
  row: TaskRemoteRepoRow;
  accessibleRepos: UseRemoteRepositoriesResult;
  selectedRepositoryIdentities: string[];
  onURLChange: RemoteRepoChipProps["onURLChange"];
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const usesTouchDrawer = useTouchDrawer();
  const portalContainer = useTaskCreateDialogPopoverContainer();
  const { t } = useTranslation();
  const triggerLabel = useMemo(() => computeTriggerLabel(row), [row]);
  const hasValue = !!row.url;
  const triggerButton = (
    <button
      type="button"
      disabled={disabled}
      onClick={usesTouchDrawer ? () => setOpen(true) : undefined}
      data-testid="remote-repo-chip-trigger"
      className={cn(
        "h-7 inline-flex items-center gap-1.5 rounded-md px-2.5 text-xs bg-transparent",
        usesTouchDrawer && "min-h-11",
        "hover:bg-muted/60 cursor-pointer",
        disabled && "cursor-not-allowed",
        !hasValue && "text-muted-foreground",
      )}
    >
      <RepoTriggerIcon row={row} />
      <span className="truncate max-w-[240px]">{triggerLabel}</span>
    </button>
  );
  const pickerContent = (
    <RemoteRepoPopoverContent
      accessible={accessibleRepos}
      selectedRepositoryIdentities={selectedRepositoryIdentities}
      onPick={(repo) => {
        onURLChange(repo.url, "picker", {
          provider: repo.provider,
          fullName: repo.fullName,
          defaultBranch: repo.defaultBranch,
          ...(repo.provider === "github" ? {} : { remoteUrl: repo.url }),
          ...(repo.providerHost ? { providerHost: repo.providerHost } : {}),
          ...(repo.providerScope ? { providerScope: repo.providerScope } : {}),
          providerRepoId: repo.id,
          providerOwner: repo.owner,
          providerName: repo.name,
        });
        setOpen(false);
      }}
      onPaste={(value) => {
        onURLChange(value, "paste");
        setOpen(false);
      }}
    />
  );
  if (usesTouchDrawer) {
    return (
      <>
        {triggerButton}
        <MobilePickerSheet
          open={open}
          onOpenChange={setOpen}
          title={t("common:repository")}
          contentTestId="remote-repo-popover-content"
        >
          {pickerContent}
        </MobilePickerSheet>
      </>
    );
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{triggerButton}</PopoverTrigger>
      <PopoverContent
        data-testid="remote-repo-popover-content"
        className="w-[380px] max-w-[calc(100vw-2rem)] max-h-[min(420px,calc(100vh-12rem))] overflow-hidden p-0"
        align="start"
        portalContainer={portalContainer}
      >
        {pickerContent}
      </PopoverContent>
    </Popover>
  );
}

function RepoTriggerIcon({ row }: { row: TaskRemoteRepoRow }) {
  if (row.source === "picker" && row.provider) {
    return <RemoteRepositoryProviderIcon provider={row.provider} />;
  }
  return <IconLink className="h-3 w-3 shrink-0 text-muted-foreground" />;
}

export function computeTriggerLabel(row: TaskRemoteRepoRow): string {
  if (!row.url) return t("task:pickOrPasteARepo");
  if (row.source === "picker" && row.fullName) return row.fullName;
  return truncateMiddle(stripScheme(row.url), TRUNCATE_THRESHOLD);
}

function stripScheme(url: string): string {
  return url.replace(/^https?:\/\//, "").replace(/^www\./, "");
}

function truncateMiddle(value: string, max: number): string {
  if (value.length <= max) return value;
  const keep = Math.max(1, Math.floor((max - 1) / 2));
  return `${value.slice(0, keep)}…${value.slice(value.length - keep)}`;
}

// --- Popover content ---------------------------------------------------------

function StagedRemoteUrlHint() {
  return (
    <div className="px-2 pt-1 text-xs text-muted-foreground">
      <Trans i18nKey="task:remoteUrlPressEnter">
        <span className="font-medium text-foreground">Remote URL</span> - press Enter to submit it.
      </Trans>
    </div>
  );
}

function RemoteRepoPopoverContent({
  accessible,
  selectedRepositoryIdentities,
  onPick,
  onPaste,
}: {
  accessible: UseRemoteRepositoriesResult;
  selectedRepositoryIdentities: string[];
  onPick: (repo: RemoteRepository) => void;
  onPaste: (value: string) => void;
}) {
  const { t } = useTranslation();
  const matchesURL = accessible.matchesURL ?? looksLikeSupportedRemoteURL;
  const isSubmittable = (value: string) => {
    const url = value.trim();
    return Boolean(parseGitHubAnyUrl(url, accessible.githubHost)) || matchesURL(url);
  };
  return (
    <RemoteRepositoryPickerPanel
      accessible={accessible}
      onPick={onPick}
      isRepoSelected={(repo) =>
        selectedRepositoryIdentities.some((identity) =>
          remoteRepositoryMatchesSelection(repo, identity),
        )
      }
      isTextSubmittable={isSubmittable}
      onSubmitText={(value): RemoteRepositoryTextSubmitResult => {
        const trimmed = value.trim();
        if (isSubmittable(trimmed)) {
          onPaste(trimmed);
          return { status: "accepted" };
        }
        if (looksLikeURL(trimmed)) {
          return { status: "rejected", message: t("task:enterRepositoryUrl") };
        }
        return { status: "ignored" };
      }}
      stagedHint={<StagedRemoteUrlHint />}
      placeholder={t("task:searchRepositoriesOrPasteARemote")}
      hideSubmitErrorWhenUnavailable
    />
  );
}
