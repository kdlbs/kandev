"use client";

import { useState } from "react";
import { IconAlertTriangle, IconLayoutDashboard, IconTrash } from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Separator } from "@kandev/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { UnsavedChangesBadge, UnsavedSaveButton } from "@/components/settings/unsaved-indicator";
import { LayoutEditor } from "./layout-editor";
import { LayoutProfileList } from "./layout-profile-list";
import { useLayoutSettings } from "./use-layout-settings";

type Controller = ReturnType<typeof useLayoutSettings>;

function LayoutSettingsHeader({ controller }: { controller: Controller }) {
  return (
    <>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <h2 className="flex items-center gap-2 text-2xl font-bold">
            <IconLayoutDashboard className="h-5 w-5" />
            Layouts
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Configure the initial desktop task workbench.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {controller.isDirty && <UnsavedChangesBadge />}
          <Button
            type="button"
            variant="outline"
            className="min-h-11 cursor-pointer sm:min-h-9"
            disabled={!controller.isDirty || controller.saveStatus === "loading"}
            onClick={controller.cancel}
          >
            Cancel
          </Button>
          <UnsavedSaveButton
            isDirty={controller.isDirty}
            isLoading={controller.saveStatus === "loading"}
            status={controller.saveStatus}
            onClick={() => void controller.save()}
            disabled={!controller.isDirty}
          />
        </div>
      </div>
      <Separator />
    </>
  );
}

function DeleteProfileButton({ onClick }: { onClick: () => void }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          size="icon-sm"
          variant="outline"
          className="min-h-11 min-w-11 cursor-pointer sm:min-h-8 sm:min-w-8"
          aria-label="Delete layout profile"
          onClick={onClick}
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>Delete layout profile</TooltipContent>
    </Tooltip>
  );
}

function SelectedLayoutHeader({
  controller,
  onDelete,
}: {
  controller: Controller;
  onDelete: () => void;
}) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0 flex-1">
        {controller.selectedCustom ? (
          <Input
            aria-label="Layout profile name"
            value={controller.selectedCustom.name}
            onChange={(event) => controller.updateSelected({ name: event.target.value })}
            className="min-h-11 max-w-md sm:min-h-9"
          />
        ) : (
          <div>
            <h3 className="text-lg font-semibold">{controller.selectedName}</h3>
            <p className="text-sm text-muted-foreground">Built-in template</p>
          </div>
        )}
      </div>
      <div className="flex flex-wrap gap-2">
        {controller.defaultActionVisible && (
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
        )}
        {controller.selectedCustom && <DeleteProfileButton onClick={onDelete} />}
      </div>
    </div>
  );
}

function SelectedLayoutEditor({ controller }: { controller: Controller }) {
  const editorKey = `${controller.selection.kind}:${controller.selection.id}:${controller.editorReset}`;
  if (!controller.editorLayout) {
    return (
      <Alert>
        <IconAlertTriangle className="h-4 w-4" />
        <AlertTitle>Visual editor unavailable</AlertTitle>
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
      editable={Boolean(controller.selectedCustom)}
      onChange={controller.updateLayout}
    />
  );
}

function DeleteProfileDialog({
  controller,
  open,
  onOpenChange,
}: {
  controller: Controller;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const confirm = () => {
    controller.deleteSelected();
    onOpenChange(false);
  };
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            Delete {controller.selectedCustom?.name ?? "layout profile"}?
          </AlertDialogTitle>
          <AlertDialogDescription>
            {controller.selectedCustom?.is_default
              ? "The built-in Default layout will become the default."
              : "This profile will be removed when you save."}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction className="cursor-pointer" onClick={confirm}>
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function LayoutSettings() {
  const controller = useLayoutSettings();
  const [deleteOpen, setDeleteOpen] = useState(false);
  return (
    <div className="min-w-0 space-y-6" data-testid="layout-settings">
      <LayoutSettingsHeader controller={controller} />
      {controller.error && (
        <Alert variant="destructive">
          <IconAlertTriangle className="h-4 w-4" />
          <AlertTitle>Layout profiles were not saved</AlertTitle>
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
        <section className="min-w-0 space-y-3" aria-label={`${controller.selectedName} editor`}>
          <SelectedLayoutHeader controller={controller} onDelete={() => setDeleteOpen(true)} />
          <SelectedLayoutEditor controller={controller} />
        </section>
      </div>
      <DeleteProfileDialog controller={controller} open={deleteOpen} onOpenChange={setDeleteOpen} />
    </div>
  );
}
