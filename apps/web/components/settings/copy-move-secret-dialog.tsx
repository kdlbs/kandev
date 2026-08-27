"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Dialog, DialogContent } from "@kandev/ui/dialog";
import { useAppStore } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import { copySecret, moveSecret } from "@/lib/api/domains/secrets-api";
import { useSecretDestinationNames } from "@/hooks/domains/settings/use-secret-destination-names";
import { useWorkspaceDestinations } from "@/hooks/domains/settings/use-workspace-destinations";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { CopyMoveDialogBody } from "./copy-move-dialog-body";

export const MAX_SECRET_NAME_BYTES = 100;

export type CopyMoveMode = "copy" | "move";

export type SecretDestination = { scope: "global" } | { scope: "workspace"; workspaceId: string };

/**
 * Truncates a string so its UTF-8 byte length is at most `limit`, iterating
 * code points (never splitting surrogate pairs or producing replacement
 * characters). The backend name limit is 100 UTF-8 bytes.
 */
export function truncateUtf8Bytes(value: string, limit: number): string {
  const encoder = new TextEncoder();
  if (encoder.encode(value).length <= limit) {
    return value;
  }
  let result = "";
  for (const char of value) {
    const next = result + char;
    if (encoder.encode(next).length > limit) {
      break;
    }
    result = next;
  }
  return result;
}

/** Default target name: `<name> (from <origin>)`, truncated to the byte limit. */
export function buildDefaultTargetName(name: string, originToken: string): string {
  return truncateUtf8Bytes(`${name} (from ${originToken})`, MAX_SECRET_NAME_BYTES);
}

type CopyMoveSecretDialogProps = {
  secret: SecretListItem;
  /** `general` for a Global source, the workspace name otherwise (literal, locale-independent). */
  originToken: string;
  /** Controlled visibility. The dialog stays mounted while closed so Radix can
   *  restore focus to the trigger on close; callers must keep `secret` set. */
  open: boolean;
  onClose: () => void;
  onCompleted: (item: SecretListItem, mode: CopyMoveMode) => void;
};

/** Renders the copy/move secret dialog, wiring form state, destinations, and submission. */
export function CopyMoveSecretDialog({
  secret,
  originToken,
  open,
  onClose,
  onCompleted,
}: CopyMoveSecretDialogProps) {
  const {
    loading: destinationsLoading,
    error: destinationsError,
    retry: retryDestinations,
  } = useWorkspaceDestinations();
  const workspaces = useAppStore((state) => state.workspaces.items);
  const [mode, setMode] = useState<CopyMoveMode>("copy");
  const [destination, setDestination] = useState<SecretDestination | null>(null);
  const [name, setName] = useState(() => buildDefaultTargetName(secret.name, originToken));
  const [nameError, setNameError] = useState<string | null>(null);
  const [destinationNamesKey, setDestinationNamesKey] = useState(0);

  const { destinations, workspaceNameById, destinationValid } = useDestinationOptions(
    secret,
    workspaces,
    destination,
    setDestination,
  );
  const destinationNames = useSecretDestinationNames(
    destination?.scope ?? "global",
    destination?.scope === "workspace" ? destination.workspaceId : undefined,
    destinationNamesKey,
  );
  const conflict = nameError === null && destination !== null && destinationNames.conflict(name);
  // The backend limit is 100 UTF-8 bytes; a user-edited name that exceeds it
  // would otherwise fail server-side with a generic transfer error.
  const nameTooLong = new TextEncoder().encode(name.trim()).length > MAX_SECRET_NAME_BYTES;

  const { busy, canSubmit, formError, clearError, run } = useTransferSubmit({
    secret,
    mode,
    destination,
    destinationValid,
    name,
    nameError,
    conflict,
    nameTooLong,
    destinationsLoading,
    destinationsError: destinationsError !== null,
    setNameError,
    onCompleted,
    open,
  });
  const resetDestinationNames = useCallback(() => setDestinationNamesKey((key) => key + 1), []);
  // Stable identity: the reset effect depends on this object, so it must not
  // change every render (the guard inside useTransferFormReset would still
  // keep behavior correct, but the effect would fire needlessly).
  const transferSetters = useMemo(
    () => ({ setMode, setDestination, setName, setNameError, clearError, resetDestinationNames }),
    [setMode, setDestination, setName, setNameError, clearError, resetDestinationNames],
  );
  useTransferFormReset(secret, originToken, open, transferSetters);
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        // While a transfer is in flight, no dismiss path (X, Escape, overlay)
        // may close the dialog: a stale completion must not land in a
        // reopened session for the same secret.
        if (!next && !busy) {
          onClose();
        }
      }}
    >
      <DialogContent
        showCloseButton={false}
        className="bottom-0 top-auto left-0 right-0 translate-x-0 translate-y-0 grid-cols-[minmax(0,1fr)] grid-rows-[auto_minmax(0,1fr)_auto] max-h-[100dvh] overflow-hidden rounded-b-none pb-[env(safe-area-inset-bottom)] sm:top-1/2 sm:left-1/2 sm:bottom-auto sm:translate-x-[-50%] sm:translate-y-[-50%] sm:rounded-b-lg"
      >
        <CopyMoveDialogBody
          secretName={secret.name}
          originToken={originToken}
          mode={mode}
          onModeChange={setMode}
          destination={destination}
          destinations={destinations}
          workspaceNameById={workspaceNameById}
          onDestinationChange={(next) => {
            // A 409 pins the name field and a non-409 failure pins the alert;
            // both are destination-specific, so clear them on target change.
            if (nameError !== null) {
              setNameError(null);
            }
            clearError();
            setDestination(next);
          }}
          destinationsLoading={destinationsLoading}
          destinationsError={destinationsError}
          onRetryDestinations={retryDestinations}
          name={name}
          onNameChange={setName}
          nameError={nameError}
          nameTooLong={nameTooLong}
          setNameError={setNameError}
          conflict={conflict}
          formError={formError}
          canSubmit={canSubmit}
          busy={busy}
          onSubmit={run}
          onClose={onClose}
        />
      </DialogContent>
    </Dialog>
  );
}

/**
 * The dialog stays mounted between opens so Radix can restore focus to the
 * trigger on close. This resets every piece of form state when a new secret is
 * targeted or when the dialog is reopened, while skipping the mount render so
 * the initial useState values (and the destination auto-selection) survive.
 */
function useTransferFormReset(
  secret: SecretListItem,
  originToken: string,
  open: boolean,
  setters: {
    setMode: (mode: CopyMoveMode) => void;
    setDestination: (destination: SecretDestination | null) => void;
    setName: (name: string) => void;
    setNameError: (error: string | null) => void;
    clearError: () => void;
    resetDestinationNames: () => void;
  },
) {
  const lastSecretId = useRef(secret.id);
  const prevOpen = useRef(open);
  useEffect(() => {
    const reopened = open && !prevOpen.current;
    prevOpen.current = open;
    if (!reopened && lastSecretId.current === secret.id) {
      return;
    }
    lastSecretId.current = secret.id;
    setters.setMode("copy");
    setters.setDestination(null);
    setters.setName(buildDefaultTargetName(secret.name, originToken));
    setters.setNameError(null);
    setters.clearError();
    setters.resetDestinationNames();
  }, [secret.id, originToken, open, setters]);
}

type TransferCanSubmitArgs = {
  isBusy: boolean;
  destination: SecretDestination | null;
  destinationValid: boolean;
  name: string;
  nameError: string | null;
  conflict: boolean;
  nameTooLong: boolean;
  destinationsLoading: boolean;
  destinationsError: boolean;
};

/** Returns whether the transfer can be submitted given the current form, destination, and request state. */
function transferCanSubmit({
  isBusy,
  destination,
  destinationValid,
  name,
  nameError,
  conflict,
  nameTooLong,
  destinationsLoading,
  destinationsError,
}: TransferCanSubmitArgs): boolean {
  return (
    !isBusy &&
    destination !== null &&
    destinationValid &&
    name.trim().length > 0 &&
    nameError === null &&
    !conflict &&
    !nameTooLong &&
    !destinationsLoading &&
    !destinationsError
  );
}

type TransferSubmitArgs = {
  secret: SecretListItem;
  mode: CopyMoveMode;
  destination: SecretDestination | null;
  destinationValid: boolean;
  name: string;
  nameError: string | null;
  conflict: boolean;
  nameTooLong: boolean;
  destinationsLoading: boolean;
  destinationsError: boolean;
  setNameError: (error: string | null) => void;
  onCompleted: (item: SecretListItem, mode: CopyMoveMode) => void;
  /** Controlled visibility; drives the session counter for stale-result guards. */
  open: boolean;
};

/** Runs the copy/move request, guarding against stale results across dialog sessions, and exposes busy/canSubmit/formError state. */
function useTransferSubmit({
  secret,
  mode,
  destination,
  destinationValid,
  name,
  nameError,
  conflict,
  nameTooLong,
  destinationsLoading,
  destinationsError,
  setNameError,
  onCompleted,
  open,
}: TransferSubmitArgs) {
  const { t } = useTranslation();
  const [isBusy, setIsBusy] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  // The dialog stays mounted across opens; a completion or error for a
  // previously targeted secret must not mutate the dialog now showing another.
  // The session counter also covers closing and reopening the SAME secret:
  // a result from the previous session must not land in the reopened dialog.
  const secretIdRef = useRef(secret.id);
  const sessionRef = useRef(0);
  useEffect(() => {
    secretIdRef.current = secret.id;
  }, [secret.id]);
  useEffect(() => {
    if (open) {
      sessionRef.current += 1;
    }
  }, [open]);

  const canSubmit = transferCanSubmit({
    isBusy,
    destination,
    destinationValid,
    name,
    nameError,
    conflict,
    nameTooLong,
    destinationsLoading,
    destinationsError,
  });

  /** Runs the transfer request, guarding stale completions against dialog reopenings. */
  const run = async () => {
    if (!canSubmit || destination === null) {
      return;
    }
    const startedSecretId = secret.id;
    const startedSession = sessionRef.current;
    setIsBusy(true);
    setFormError(null);
    const trimmedName = name.trim();
    const payload = {
      target_scope: destination.scope,
      ...(destination.scope === "workspace"
        ? { target_workspace_id: destination.workspaceId }
        : {}),
      name: trimmedName,
    } as const;
    try {
      // A workspace-scoped source requires its workspace id in the query.
      const sourceOptions =
        secret.scope === "workspace" && secret.workspace_id
          ? { workspaceId: secret.workspace_id }
          : undefined;
      const item =
        mode === "copy"
          ? await copySecret(secret.id, payload, sourceOptions)
          : await moveSecret(secret.id, payload, sourceOptions);
      if (secretIdRef.current === startedSecretId && sessionRef.current === startedSession) {
        onCompleted(item, mode);
      }
    } catch (err) {
      // A stale failure (user switched to another secret mid-flight, or closed
      // and reopened the same one) must not paint onto the current dialog.
      if (secretIdRef.current !== startedSecretId || sessionRef.current !== startedSession) {
        return;
      }
      if (err instanceof ApiError && err.status === 409) {
        setNameError(t("settings:secretNameConflictInDestination", { name: trimmedName }));
      } else {
        // 404 is deliberately ambiguous (missing source OR missing/unauthorized
        // destination) to avoid disclosure; never remove the source row on it.
        setFormError(t("settings:transferFailed"));
      }
    } finally {
      setIsBusy(false);
    }
  };

  const clearError = useCallback(() => setFormError(null), []);

  return { busy: isBusy, canSubmit, formError, clearError, run };
}

type WorkspaceOption = { id: string; name: string };

/**
 * Computes the valid destinations for a secret (General plus every other
 * workspace) and keeps the selection in sync: auto-selects the first option
 * when nothing is chosen, and clears a selection that vanished from the list
 * (deleted workspace or failed list) so submission can never target a stale
 * destination. `null` keeps canSubmit false when nothing valid remains (a
 * Global source must never fall back to a same-scope target).
 */
function useDestinationOptions(
  secret: SecretListItem,
  workspaces: WorkspaceOption[],
  destination: SecretDestination | null,
  onDestinationChange: (destination: SecretDestination | null) => void,
) {
  const workspaceNameById = Object.fromEntries(
    workspaces.map((workspace) => [workspace.id, workspace.name]),
  );
  const destinations = useMemo(() => {
    const list: SecretDestination[] = [];
    if (secret.scope !== "global") {
      list.push({ scope: "global" });
    }
    for (const workspace of workspaces) {
      if (secret.scope === "workspace" && workspace.id === secret.workspace_id) {
        continue;
      }
      list.push({ scope: "workspace", workspaceId: workspace.id });
    }
    return list;
  }, [secret, workspaces]);

  // Synchronous validity: the selected destination must be present in the
  // CURRENT list. canSubmit consumes this on the same render, so a workspace
  // that vanished (deleted or failed list) can never be submitted even
  // between renders; the effect below only performs state cleanup.
  const destinationValid =
    destination !== null &&
    destinations.some((option) => {
      if (option.scope !== destination.scope) {
        return false;
      }
      if (destination.scope === "workspace") {
        return option.scope === "workspace" && option.workspaceId === destination.workspaceId;
      }
      return true;
    });

  useEffect(() => {
    if (destination === null && destinations.length > 0) {
      onDestinationChange(destinations[0]);
      return;
    }
    if (destination !== null && !destinationValid) {
      // The selected destination disappeared from the list: clear it so the
      // UI cannot show a stale selection. null keeps canSubmit false when
      // nothing valid remains (a Global source must never fall back to a
      // same-scope target).
      onDestinationChange(destinations[0] ?? null);
    }
  }, [destination, destinations, destinationValid, onDestinationChange]);

  return { destinations, workspaceNameById, destinationValid };
}
