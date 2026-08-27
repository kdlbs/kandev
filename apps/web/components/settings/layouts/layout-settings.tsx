"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { IconAlertTriangle, IconLayoutDashboard, IconRestore } from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Input } from "@kandev/ui/input";
import { Separator } from "@kandev/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { LayoutEditor } from "./layout-editor";
import { LayoutProfileList } from "./layout-profile-list";
import { LayoutProfileDeleteConfirmation } from "./layout-profile-delete-confirmation";
import { useLayoutSettings } from "./use-layout-settings";
import { useTranslation } from "react-i18next";
import { SettingsTarget } from "@/components/settings/settings-target";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";

type Controller = ReturnType<typeof useLayoutSettings>;

/** Returns a catalog key; the caller resolves it so the copy follows locale switches. */
function defaultActionHelpKey(selectedSavedDefault: boolean, selectedIsDefault: boolean) {
  if (selectedSavedDefault) return "settings:makeTheOriginalDefaultLayoutThe";
  if (selectedIsDefault) return "settings:thisLayoutIsUsedAsThe";
  return "settings:useThisLayoutAsTheStarting";
}

function LayoutSettingsHeader() {
  const { t } = useTranslation();
  return (
    <>
      <div className="min-w-0">
        <h2 className="flex items-center gap-2 text-2xl font-bold">
          <IconLayoutDashboard className="h-5 w-5" />
          {t("settings:layouts")}
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">
          {t("settings:configureTheInitialDesktopTaskWorkbench")}
        </p>
      </div>
      <Separator />
    </>
  );
}

function ResetBuiltInButton({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="min-h-11 cursor-pointer sm:min-h-8"
          aria-label={t("settings:resetBuiltInLayout")}
          onClick={onClick}
        >
          <IconRestore className="h-4 w-4" /> {t("settings:reset")}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("settings:restoreTheOriginalBuiltInLayout")}</TooltipContent>
    </Tooltip>
  );
}

function SelectedLayoutHeader({
  controller,
  deleteAction,
}: {
  controller: Controller;
  deleteAction: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0 flex-1">
        {controller.selectedCustom ? (
          <Input
            aria-label={t("settings:layoutProfileName")}
            value={controller.selectedCustom.name}
            onChange={(event) => controller.updateSelected({ name: event.target.value })}
            className="min-h-11 max-w-md sm:min-h-9"
          />
        ) : (
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-lg font-semibold">{controller.selectedName}</h3>
            <Badge variant="outline">{t("settings:builtIn")}</Badge>
            {controller.selectedBuiltInOverride && (
              <Badge variant="secondary">{t("settings:customized")}</Badge>
            )}
          </div>
        )}
      </div>
      <div className="flex flex-wrap gap-2">
        <Tooltip>
          <TooltipTrigger asChild>
            <span tabIndex={controller.defaultActionDisabled ? 0 : -1} className="inline-flex">
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="min-h-11 cursor-pointer sm:min-h-8"
                disabled={controller.defaultActionDisabled}
                onClick={controller.setDefault}
              >
                {controller.defaultActionLabel}
              </Button>
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {t(defaultActionHelpKey(controller.selectedSavedDefault, controller.selectedIsDefault))}
          </TooltipContent>
        </Tooltip>
        {controller.selectedBuiltInOverride && (
          <ResetBuiltInButton onClick={controller.resetBuiltIn} />
        )}
        {deleteAction}
      </div>
    </div>
  );
}

function SelectedLayoutEditor({ controller }: { controller: Controller }) {
  const { t } = useTranslation();
  const editorKey = `${controller.selection.kind}:${controller.selection.id}:${controller.editorReset}`;
  if (!controller.editorLayout) {
    return (
      <Alert>
        <IconAlertTriangle className="h-4 w-4" />
        <AlertTitle>{t("settings:visualEditorUnavailable")}</AlertTitle>
        <AlertDescription>
          {controller.compatibility?.issues.map((issue) => issue.message).join(". ")}
        </AlertDescription>
      </Alert>
    );
  }
  return (
    <LayoutEditor
      key={editorKey}
      layout={controller.editorLayout}
      editable
      onChange={controller.updateLayout}
    />
  );
}

export function LayoutSettings() {
  const { t } = useTranslation();
  const controller = useLayoutSettings();
  const { isFinePointer } = useResponsiveBreakpoint();
  const deleteAnchorRef = useRef<HTMLButtonElement>(null);
  const [deleteProfileId, setDeleteProfileId] = useState<string | null>(null);
  const selectedCustomId = controller.selectedCustom?.id ?? null;
  const deleteOpen = deleteProfileId !== null && deleteProfileId === selectedCustomId;

  useEffect(() => {
    if (deleteProfileId && deleteProfileId !== selectedCustomId) setDeleteProfileId(null);
  }, [deleteProfileId, selectedCustomId]);

  const requestDelete = () => {
    if (selectedCustomId) setDeleteProfileId(selectedCustomId);
  };
  const closeDelete = () => setDeleteProfileId(null);
  const confirmDelete = () => {
    if (!deleteOpen) return;
    closeDelete();
    controller.deleteSelected();
  };
  const invalidName = controller.profiles.some((profile) => !profile.name.trim());
  useSettingsSaveContributor({
    id: "layout-profiles",
    revision: controller.profilesKey,
    isDirty: controller.isDirty,
    canSave: !invalidName,
    invalidReason: invalidName ? t("settings:layoutProfileNamesMustNotBe") : undefined,
    save: controller.save,
    discard: controller.cancel,
  });
  return (
    <SettingsTarget
      targetId={GENERAL_SETTINGS_TARGETS.layoutProfiles}
      className="min-w-0 space-y-6"
      data-testid="layout-settings"
    >
      <LayoutSettingsHeader />
      {controller.error && (
        <Alert variant="destructive">
          <IconAlertTriangle className="h-4 w-4" />
          <AlertTitle>{t("settings:layoutProfilesWereNotSaved")}</AlertTitle>
          <AlertDescription>{controller.error}</AlertDescription>
        </Alert>
      )}
      <div className="grid min-w-0 gap-5 lg:grid-cols-[16rem_minmax(0,1fr)]">
        <LayoutProfileList
          profiles={controller.profiles}
          selection={controller.selection}
          onSelect={controller.setSelection}
          onCreate={controller.create}
          onDuplicate={controller.duplicate}
        />
        <section
          className="min-w-0 space-y-3"
          aria-label={t("settings:layoutEditorForProfile", { name: controller.selectedName })}
        >
          <SelectedLayoutHeader
            controller={controller}
            deleteAction={
              controller.selectedCustom ? (
                <LayoutProfileDeleteConfirmation
                  profile={controller.selectedCustom}
                  isFinePointer={isFinePointer}
                  open={deleteOpen}
                  anchorRef={deleteAnchorRef}
                  onOpenChange={(open) => (open ? requestDelete() : closeDelete())}
                  onConfirm={confirmDelete}
                />
              ) : null
            }
          />
          <SelectedLayoutEditor controller={controller} />
        </section>
      </div>
    </SettingsTarget>
  );
}
