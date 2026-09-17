"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconCheck, IconExternalLink } from "@tabler/icons-react";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Drawer, DrawerContent, DrawerFooter } from "@kandev/ui/drawer";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type {
  WorkflowImportPreview,
  WorkflowImportProfileCandidate,
  WorkflowImportProfileConflict,
  WorkflowImportProfileStep,
} from "@/lib/types/http";
import {
  workflowImportProfileLabel,
  workflowImportStepKey,
  type WorkflowImportSelections,
} from "./use-workflow-import";
import { MobileSelectionHeader } from "./workflow-import-profile-selection-mobile-header";

type WorkflowImportProfileSelectionProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  preview: WorkflowImportPreview;
  selections: WorkflowImportSelections;
  missingSteps: WorkflowImportProfileStep[];
  profileConflicts: WorkflowImportProfileConflict[];
  activeStepKey: string | null;
  onActiveStepKeyChange: (key: string | null) => void;
  onSelectProfile: (stepKey: string, profileId: string) => void;
  onImport: () => void | Promise<void>;
  onRetryPreview: () => void | Promise<void>;
  importLoading: boolean;
};

function profileField(value: string, translate: (key: string) => string): string {
  return value || translate("workflows:importProfileNotSet");
}

function ProfileDetails({
  profile,
  compact = false,
}: {
  profile: WorkflowImportProfileCandidate;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <span className={compact ? "flex min-w-0 flex-col text-left" : "flex min-w-0 flex-col gap-1"}>
      <span className="truncate font-medium">{profile.name || profile.id}</span>
      <span className="truncate text-xs text-muted-foreground">
        {profileField(profile.agent_name, t)} · {profileField(profile.model, t)} ·{" "}
        {profileField(profile.mode, t)}
      </span>
    </span>
  );
}

function RequestedProfileDetails({ step }: { step: WorkflowImportProfileStep }) {
  const { t } = useTranslation();
  const requested = step.requested_profile;
  return (
    <span className="min-w-0 text-xs text-muted-foreground">
      <span className="mr-1 font-medium text-foreground">
        {t("workflows:importRequestedProfile")}
      </span>
      {profileField(requested.agent_name, t)} · {profileField(requested.model ?? "", t)} ·{" "}
      {profileField(requested.mode ?? "", t)}
    </span>
  );
}

function ProfilePickerList({
  profiles,
  selectedProfileId,
  onSelect,
}: {
  profiles: WorkflowImportProfileCandidate[];
  selectedProfileId?: string;
  onSelect: (profileId: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Command shouldFilter>
      <CommandInput
        autoFocus
        placeholder={t("workflows:importProfileSearchPlaceholder")}
        aria-label={t("workflows:importProfileSearchPlaceholder")}
      />
      <CommandList className="max-h-[min(50vh,24rem)] overscroll-contain">
        <CommandEmpty>{t("workflows:importProfileNoMatches")}</CommandEmpty>
        <CommandGroup>
          {profiles.map((profile) => (
            <CommandItem
              key={profile.id}
              value={workflowImportProfileLabel(profile)}
              keywords={[profile.id, profile.name, profile.agent_name, profile.model, profile.mode]}
              onSelect={() => onSelect(profile.id)}
              className="min-h-11 cursor-pointer gap-2"
              data-testid={`workflow-import-profile-option-${profile.id}`}
            >
              <IconCheck
                className={
                  selectedProfileId === profile.id
                    ? "h-4 w-4 shrink-0"
                    : "h-4 w-4 shrink-0 opacity-0"
                }
                aria-hidden="true"
              />
              <ProfileDetails profile={profile} compact />
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </Command>
  );
}

function conflictForStep(
  conflicts: WorkflowImportProfileConflict[],
  step: WorkflowImportProfileStep,
): WorkflowImportProfileConflict | undefined {
  const key = workflowImportStepKey(step);
  return conflicts.find((conflict) => workflowImportStepKey(conflict) === key);
}

function ConflictMessage({ conflict }: { conflict?: WorkflowImportProfileConflict }) {
  const { t } = useTranslation();
  if (!conflict) return null;
  const messageKey =
    {
      changed_profile: "workflows:importProfileChanged",
      unavailable_profile: "workflows:importProfileUnavailable",
      missing_selection: "workflows:importProfileSelectionRequired",
    }[conflict.reason] ?? "workflows:importProfileSelectionRequired";
  return (
    <p className="flex items-start gap-1.5 text-xs text-destructive" role="alert">
      <IconAlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
      <span>{t(messageKey)}</span>
    </p>
  );
}

function SelectionRow({
  step,
  preview,
  selections,
  conflicts,
  mobile,
  open,
  onOpenChange,
  onSelectProfile,
}: {
  step: WorkflowImportProfileStep;
  preview: WorkflowImportPreview;
  selections: WorkflowImportSelections;
  conflicts: WorkflowImportProfileConflict[];
  mobile: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelectProfile: (profileId: string) => void;
}) {
  const { t } = useTranslation();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const key = workflowImportStepKey(step);
  const selectedProfileId = selections[key];
  const selectedProfile = preview.profiles.find((profile) => profile.id === selectedProfileId);
  const isAutomatic = step.matched_profile?.id === selectedProfileId;
  const conflict = conflictForStep(conflicts, step);
  const profileButton = (
    <Button
      type="button"
      variant="outline"
      ref={triggerRef}
      disabled={preview.profiles.length === 0}
      onClick={mobile ? () => onOpenChange(true) : undefined}
      className="min-h-11 w-full cursor-pointer justify-start text-left sm:min-h-7"
      data-testid={`workflow-import-profile-select-${key}`}
    >
      {selectedProfile ? (
        <ProfileDetails profile={selectedProfile} compact />
      ) : (
        t("workflows:importSelectProfile")
      )}
    </Button>
  );
  let profileContent = profileButton;
  if (!mobile) {
    profileContent = (
      <Popover modal={false} open={open} onOpenChange={onOpenChange}>
        <PopoverTrigger asChild>{profileButton}</PopoverTrigger>
        <PopoverContent
          align="start"
          className="w-[min(24rem,calc(100vw-2rem))] p-0"
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            triggerRef.current?.focus({ preventScroll: true });
          }}
        >
          <ProfilePickerList
            profiles={preview.profiles}
            selectedProfileId={selectedProfileId}
            onSelect={onSelectProfile}
          />
        </PopoverContent>
      </Popover>
    );
  }
  return (
    <div
      className="space-y-2 rounded-lg border border-border/70 p-3"
      data-testid={`workflow-import-profile-step-${key}`}
    >
      <div className="min-w-0 space-y-1">
        <p className="truncate font-medium">{step.step_name}</p>
        <RequestedProfileDetails step={step} />
      </div>
      {isAutomatic && selectedProfile ? (
        <div
          className="flex min-w-0 items-center gap-2 rounded-md bg-muted/50 p-2 text-sm"
          data-testid={`workflow-import-profile-match-${key}`}
        >
          <IconCheck className="h-4 w-4 shrink-0 text-green-600" aria-hidden="true" />
          <ProfileDetails profile={selectedProfile} compact />
          <span className="ml-auto shrink-0 text-xs text-muted-foreground">
            {t("workflows:importProfileMatched")}
          </span>
        </div>
      ) : (
        profileContent
      )}
      <ConflictMessage conflict={conflict} />
    </div>
  );
}

function SelectionRows({
  preview,
  selections,
  conflicts,
  mobile,
  openStepKey,
  onOpenStep,
  onSelectProfile,
}: {
  preview: WorkflowImportPreview;
  selections: WorkflowImportSelections;
  conflicts: WorkflowImportProfileConflict[];
  mobile: boolean;
  openStepKey: string | null;
  onOpenStep: (key: string | null) => void;
  onSelectProfile: (stepKey: string, profileId: string) => void;
}) {
  const groups = new Map<number, { name: string; steps: WorkflowImportProfileStep[] }>();
  for (const step of preview.steps) {
    const group = groups.get(step.workflow_index) ?? { name: step.workflow_name, steps: [] };
    group.steps.push(step);
    groups.set(step.workflow_index, group);
  }
  return (
    <div className="space-y-4">
      {Array.from(groups.entries()).map(([workflowIndex, group]) => (
        <section
          key={workflowIndex}
          className="space-y-2"
          data-testid={`workflow-import-group-${workflowIndex}`}
        >
          <h3 className="font-medium">{group.name}</h3>
          <div className="space-y-2">
            {group.steps.map((step) => {
              const key = workflowImportStepKey(step);
              return (
                <SelectionRow
                  key={key}
                  step={step}
                  preview={preview}
                  selections={selections}
                  conflicts={conflicts}
                  mobile={mobile}
                  open={openStepKey === key}
                  onOpenChange={(open) => onOpenStep(open ? key : null)}
                  onSelectProfile={(profileId) => onSelectProfile(key, profileId)}
                />
              );
            })}
          </div>
        </section>
      ))}
    </div>
  );
}

function EmptyProfileCallout({ onRetry }: { onRetry: () => void | Promise<void> }) {
  const { t } = useTranslation();
  return (
    <div
      className="space-y-3 rounded-lg border border-dashed border-border p-4"
      data-testid="workflow-import-no-profiles"
    >
      <p className="text-sm text-muted-foreground">{t("workflows:importNoEligibleProfiles")}</p>
      <div className="flex flex-wrap items-center gap-3">
        <a
          href="/settings/agents"
          target="_blank"
          rel="noreferrer"
          className="inline-flex min-h-11 cursor-pointer items-center gap-1 text-sm text-primary underline-offset-4 hover:underline sm:min-h-7"
        >
          {t("workflows:openAgentSettings")}
          <IconExternalLink className="h-4 w-4" aria-hidden="true" />
        </a>
        <Button
          type="button"
          variant="outline"
          className="min-h-11 cursor-pointer sm:min-h-7"
          onClick={onRetry}
        >
          {t("workflows:retryImportPreview")}
        </Button>
      </div>
    </div>
  );
}

function SelectionFooter({
  canImport,
  importLoading,
  showRetry,
  onOpenChange,
  onImport,
  onRetry,
}: {
  canImport: boolean;
  importLoading: boolean;
  showRetry: boolean;
  onOpenChange: (open: boolean) => void;
  onImport: () => void | Promise<void>;
  onRetry: () => void | Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Button
        type="button"
        variant="outline"
        onClick={() => onOpenChange(false)}
        disabled={importLoading}
        className="min-h-11 cursor-pointer sm:min-h-7"
      >
        {t("common:cancel")}
      </Button>
      {showRetry && (
        <Button
          type="button"
          variant="outline"
          onClick={onRetry}
          disabled={importLoading}
          className="min-h-11 cursor-pointer sm:min-h-7"
        >
          {t("workflows:retryImportPreview")}
        </Button>
      )}
      <Button
        type="button"
        onClick={onImport}
        disabled={!canImport || importLoading}
        className="min-h-11 cursor-pointer sm:min-h-7"
        data-testid="workflow-import-profile-submit"
      >
        <span role="status" aria-live="polite">
          {importLoading ? t("workflows:importing") : t("workflows:importContinue")}
        </span>
      </Button>
    </>
  );
}

function MobileSelectionBody({
  mobileView,
  activeStep,
  activeStepKey,
  preview,
  selections,
  missingSteps,
  profileConflicts,
  onRetryPreview,
  onOpenMobilePicker,
  onSelectMobileProfile,
  onSelectProfile,
}: {
  mobileView: "list" | "picker";
  activeStep?: WorkflowImportProfileStep;
  activeStepKey: string | null;
  preview: WorkflowImportPreview;
  selections: WorkflowImportSelections;
  missingSteps: WorkflowImportProfileStep[];
  profileConflicts: WorkflowImportProfileConflict[];
  onRetryPreview: () => void | Promise<void>;
  onOpenMobilePicker: (key: string | null) => void;
  onSelectMobileProfile: (profileId: string) => void;
  onSelectProfile: (stepKey: string, profileId: string) => void;
}) {
  if (mobileView === "picker" && activeStep) {
    return (
      <ProfilePickerList
        profiles={preview.profiles}
        selectedProfileId={selections[activeStepKey ?? ""]}
        onSelect={onSelectMobileProfile}
      />
    );
  }
  return (
    <>
      {preview.profiles.length === 0 && missingSteps.length > 0 && (
        <EmptyProfileCallout onRetry={onRetryPreview} />
      )}
      <SelectionRows
        preview={preview}
        selections={selections}
        conflicts={profileConflicts}
        mobile
        openStepKey={null}
        onOpenStep={onOpenMobilePicker}
        onSelectProfile={onSelectProfile}
      />
    </>
  );
}

function MobileProfileSelection({
  open,
  onOpenChange,
  preview,
  selections,
  missingSteps,
  profileConflicts,
  activeStepKey,
  onActiveStepKeyChange,
  onSelectProfile,
  onImport,
  onRetryPreview,
  importLoading,
}: WorkflowImportProfileSelectionProps) {
  const [mobileView, setMobileView] = useState<"list" | "picker">("list");
  useEffect(() => {
    if (open) setMobileView("list");
  }, [open]);
  const activeStep = preview.steps.find((step) => workflowImportStepKey(step) === activeStepKey);
  const openMobilePicker = (key: string | null) => {
    onActiveStepKeyChange(key);
    if (key) setMobileView("picker");
    else setMobileView("list");
  };
  const selectMobileProfile = (profileId: string) => {
    if (!activeStepKey) return;
    onSelectProfile(activeStepKey, profileId);
    setMobileView("list");
  };
  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerContent className="h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] !max-h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] outline-none">
        <div
          className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl bg-background shadow-2xl shadow-black/20"
          data-testid="workflow-import-profile-selection"
        >
          <MobileSelectionHeader
            mobileView={mobileView}
            activeStep={activeStep}
            onBack={() => openMobilePicker(null)}
          />
          <div
            className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4 pb-[env(safe-area-inset-bottom,0px)]"
            aria-live="polite"
          >
            <MobileSelectionBody
              mobileView={mobileView}
              activeStep={activeStep}
              activeStepKey={activeStepKey}
              preview={preview}
              selections={selections}
              missingSteps={missingSteps}
              profileConflicts={profileConflicts}
              onRetryPreview={onRetryPreview}
              onOpenMobilePicker={openMobilePicker}
              onSelectMobileProfile={selectMobileProfile}
              onSelectProfile={onSelectProfile}
            />
          </div>
          <DrawerFooter className="shrink-0 border-t border-border/70 bg-background/95 pb-[calc(1rem+env(safe-area-inset-bottom,0px))]">
            <SelectionFooter
              canImport={missingSteps.length === 0}
              importLoading={importLoading}
              showRetry={
                profileConflicts.length > 0 ||
                (preview.profiles.length === 0 && missingSteps.length > 0)
              }
              onOpenChange={onOpenChange}
              onImport={onImport}
              onRetry={onRetryPreview}
            />
          </DrawerFooter>
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function DesktopProfileSelection({
  open,
  onOpenChange,
  preview,
  selections,
  missingSteps,
  profileConflicts,
  activeStepKey,
  onActiveStepKeyChange,
  onSelectProfile,
  onImport,
  onRetryPreview,
  importLoading,
}: WorkflowImportProfileSelectionProps) {
  const { t } = useTranslation();
  const canImport = missingSteps.length === 0;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="flex max-h-[min(88vh,48rem)] flex-col overflow-hidden sm:max-w-2xl"
        data-testid="workflow-import-profile-selection"
      >
        <DialogHeader>
          <DialogTitle>{t("workflows:importResolveProfilesTitle")}</DialogTitle>
          <p className="text-sm text-muted-foreground">
            {t("workflows:importResolveProfilesDescription")}
          </p>
        </DialogHeader>
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain" aria-live="polite">
          {preview.profiles.length === 0 && missingSteps.length > 0 && (
            <EmptyProfileCallout onRetry={onRetryPreview} />
          )}
          <SelectionRows
            preview={preview}
            selections={selections}
            conflicts={profileConflicts}
            mobile={false}
            openStepKey={activeStepKey}
            onOpenStep={onActiveStepKeyChange}
            onSelectProfile={onSelectProfile}
          />
        </div>
        <DialogFooter>
          <SelectionFooter
            canImport={canImport}
            importLoading={importLoading}
            showRetry={profileConflicts.length > 0}
            onOpenChange={onOpenChange}
            onImport={onImport}
            onRetry={onRetryPreview}
          />
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function WorkflowImportProfileSelection(props: WorkflowImportProfileSelectionProps) {
  const { isMobile } = useResponsiveBreakpoint();
  return isMobile ? <MobileProfileSelection {...props} /> : <DesktopProfileSelection {...props} />;
}
