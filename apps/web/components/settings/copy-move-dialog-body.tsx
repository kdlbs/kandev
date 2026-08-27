"use client";

import { useTranslation } from "react-i18next";
import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DialogClose,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { CopyMoveMode, SecretDestination } from "./copy-move-secret-dialog";

/** Renders the Copy/Move dialog body: mode selector, destination picker, target name, and footer actions. */
export function CopyMoveDialogBody({
  secretName,
  originToken,
  mode,
  onModeChange,
  destination,
  destinations,
  workspaceNameById,
  onDestinationChange,
  destinationsLoading,
  destinationsError,
  onRetryDestinations,
  name,
  onNameChange,
  nameError,
  setNameError,
  conflict,
  nameTooLong,
  formError,
  canSubmit,
  busy,
  onSubmit,
  onClose,
}: {
  secretName: string;
  originToken: string;
  mode: CopyMoveMode;
  onModeChange: (mode: CopyMoveMode) => void;
  destination: SecretDestination | null;
  destinations: SecretDestination[];
  workspaceNameById: Record<string, string>;
  onDestinationChange: (destination: SecretDestination) => void;
  destinationsLoading: boolean;
  destinationsError: string | null;
  onRetryDestinations: () => void;
  name: string;
  onNameChange: (name: string) => void;
  nameError: string | null;
  setNameError: (error: string | null) => void;
  conflict: boolean;
  nameTooLong: boolean;
  formError: string | null;
  canSubmit: boolean;
  busy: boolean;
  onSubmit: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <DialogHeader>
        <DialogTitle className="min-w-0 break-words">
          {t("settings:copyMoveSecretNamed", { name: secretName })}
        </DialogTitle>
        <DialogDescription>{t("settings:copyMoveDialogHint")}</DialogDescription>
      </DialogHeader>

      {/* The middle section scrolls on short viewports; the header and the
          footer stay pinned so actions are never pushed off-screen. */}
      <div className="min-h-0 space-y-4 overflow-y-auto">
        <TransferModeField mode={mode} onModeChange={onModeChange} originToken={originToken} />

        <DestinationField
          destinations={destinations}
          workspaceNameById={workspaceNameById}
          destination={destination}
          onDestinationChange={onDestinationChange}
          destinationsLoading={destinationsLoading}
          destinationsError={destinationsError}
          onRetryDestinations={onRetryDestinations}
          disabled={busy}
        />
        <TargetNameField
          name={name}
          onNameChange={onNameChange}
          nameError={nameError}
          setNameError={setNameError}
          conflict={conflict}
          nameTooLong={nameTooLong}
          disabled={busy}
        />
        {formError !== null && (
          <p className="text-xs text-destructive" role="alert">
            {formError}
          </p>
        )}
      </div>
      <TransferDialogFooter
        mode={mode}
        canSubmit={canSubmit}
        busy={busy}
        onSubmit={onSubmit}
        onClose={onClose}
      />
    </>
  );
}

/** Renders the dialog footer with cancel, submit, and close actions for a copy/move transfer. */
function TransferDialogFooter({
  mode,
  canSubmit,
  busy,
  onSubmit,
  onClose,
}: {
  mode: CopyMoveMode;
  canSubmit: boolean;
  busy: boolean;
  onSubmit: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <DialogFooter>
      <Button
        type="button"
        variant="outline"
        onClick={onClose}
        disabled={busy}
        className="min-h-11 cursor-pointer"
      >
        {t("settings:cancel")}
      </Button>
      <Button
        type="button"
        onClick={onSubmit}
        disabled={!canSubmit}
        className="min-h-11 cursor-pointer"
      >
        {mode === "copy" ? t("settings:copySecretAction") : t("settings:moveSecretAction")}
      </Button>
      <DialogClose asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label={t("common:close")}
          disabled={busy}
          className="absolute top-2 right-2 min-h-11 min-w-11 cursor-pointer"
        >
          <IconX className="h-4 w-4" />
        </Button>
      </DialogClose>
    </DialogFooter>
  );
}

/** Renders the target-name input with conflict and validation error display. */
function TargetNameField({
  name,
  onNameChange,
  nameError,
  setNameError,
  conflict,
  nameTooLong,
  disabled,
}: {
  name: string;
  onNameChange: (name: string) => void;
  nameError: string | null;
  setNameError: (error: string | null) => void;
  conflict: boolean;
  nameTooLong: boolean;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const invalid = conflict || nameError !== null || nameTooLong;
  return (
    <div className="space-y-2">
      <Label htmlFor="copy-move-name">{t("settings:secretTargetName")}</Label>
      <Input
        id="copy-move-name"
        value={name}
        onChange={(e) => {
          onNameChange(e.target.value);
          if (nameError !== null) {
            setNameError(null);
          }
        }}
        disabled={disabled}
        aria-invalid={invalid}
        aria-describedby={invalid ? "copy-move-name-error" : undefined}
        className="min-h-11"
      />
      {invalid && (
        <p id="copy-move-name-error" className="text-xs text-destructive break-words">
          {nameError ??
            (nameTooLong
              ? t("settings:secretNameTooLong")
              : t("settings:secretNameConflictInDestination", { name: name.trim() }))}
        </p>
      )}
    </div>
  );
}

/** Renders the copy/move mode radio group, including the move warning. */
function TransferModeField({
  mode,
  onModeChange,
  originToken,
}: {
  mode: CopyMoveMode;
  onModeChange: (mode: CopyMoveMode) => void;
  originToken: string;
}) {
  const { t } = useTranslation();
  return (
    <fieldset className="space-y-2">
      <legend className="sr-only">{t("settings:copyMoveMode")}</legend>
      <RadioGroup
        value={mode}
        onValueChange={(value) => onModeChange(value as CopyMoveMode)}
        className="flex flex-col gap-2"
      >
        <label className="flex items-start gap-2 rounded-lg border border-border/70 p-3 min-h-11 cursor-pointer">
          <RadioGroupItem value="copy" className="mt-0.5" />
          <span className="space-y-1">
            <span className="block text-sm font-medium">{t("settings:copySecretAction")}</span>
            <span className="block text-xs text-muted-foreground">
              {t("settings:copyModeDescription")}
            </span>
          </span>
        </label>
        <label className="flex items-start gap-2 rounded-lg border border-border/70 p-3 min-h-11 cursor-pointer">
          <RadioGroupItem value="move" className="mt-0.5" />
          <span className="space-y-1">
            <span className="block text-sm font-medium">{t("settings:moveSecretAction")}</span>
            <span className="block text-xs text-muted-foreground">
              {t("settings:moveModeWarning", { origin: originToken })}
            </span>
          </span>
        </label>
      </RadioGroup>
    </fieldset>
  );
}

/** Renders the destination picker with loading, error/retry, and empty states. */
function DestinationField({
  destinations,
  workspaceNameById,
  destination,
  onDestinationChange,
  destinationsLoading,
  destinationsError,
  onRetryDestinations,
  disabled,
}: {
  destinations: SecretDestination[];
  workspaceNameById: Record<string, string>;
  destination: SecretDestination | null;
  onDestinationChange: (destination: SecretDestination) => void;
  destinationsLoading: boolean;
  destinationsError: string | null;
  onRetryDestinations: () => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const noDestinations =
    destinations.length === 0 && !destinationsLoading && destinationsError === null;

  return (
    <div className="space-y-2">
      <Label htmlFor="copy-move-destination">{t("settings:destination")}</Label>
      {destinationsLoading && (
        <p className="text-xs text-muted-foreground">{t("settings:destinationsLoading")}</p>
      )}
      {destinationsError !== null && (
        <div className="flex items-center gap-2">
          <p className="text-xs text-destructive">{t("settings:destinationsLoadFailed")}</p>
          <Button
            variant="outline"
            size="sm"
            onClick={onRetryDestinations}
            className="min-h-11 cursor-pointer"
          >
            {t("settings:retryDestinations")}
          </Button>
        </div>
      )}
      {noDestinations && (
        <p className="text-xs text-destructive">{t("settings:noValidDestinations")}</p>
      )}
      {destination !== null && (
        <Select
          value={destination.scope === "global" ? "global" : `workspace:${destination.workspaceId}`}
          onValueChange={(value) =>
            onDestinationChange(
              value === "global"
                ? { scope: "global" }
                : { scope: "workspace", workspaceId: value.slice("workspace:".length) },
            )
          }
        >
          <SelectTrigger
            id="copy-move-destination"
            disabled={disabled}
            className="min-h-11 w-full cursor-pointer"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {destinations.map((option) =>
              option.scope === "global" ? (
                <SelectItem key="global" value="global">
                  {t("settings:destinationGlobal")}
                </SelectItem>
              ) : (
                <SelectItem key={option.workspaceId} value={`workspace:${option.workspaceId}`}>
                  {workspaceNameById[option.workspaceId] ?? option.workspaceId}
                </SelectItem>
              ),
            )}
          </SelectContent>
        </Select>
      )}
    </div>
  );
}
