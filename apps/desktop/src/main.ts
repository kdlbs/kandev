import { invoke } from "@tauri-apps/api/core";
import "./styles.css";
import { getStartupText, startupLocale, type StartupCopyKey } from "./startup-copy";

type StartupConflict = {
  target_kind: "home" | "database";
  target_path: string;
  storage_kind: "sqlite_in_home" | "sqlite_external" | "postgres";
  database_path?: string;
  owner?: {
    pid?: number;
    executable?: string;
    started_at?: string;
  };
};

type DesktopStatus = {
  kind: "loading" | "temporary" | "failure" | "conflict";
  detail?: string;
  home?: string;
  conflict?: StartupConflict;
};

declare global {
  interface Window {
    __KANDEV_DESKTOP_SET_STATUS?: (status: DesktopStatus) => void;
    __KANDEV_DESKTOP_PENDING_STATUS?: DesktopStatus;
  }
}

const shell = document.querySelector<HTMLElement>("#startup-shell");
const loadingState = document.querySelector<HTMLElement>("#loading-state");
const loadingTitle = document.querySelector<HTMLHeadingElement>("#status-title");
const loadingDetail = document.querySelector<HTMLParagraphElement>("#status-detail");
const loadingHome = document.querySelector<HTMLElement>("#temporary-home");
const loadingHomePath = document.querySelector<HTMLElement>("#temporary-home-path");
const spinnerStatus = document.querySelector<HTMLElement>("#spinner-status");
const conflictPanel = document.querySelector<HTMLElement>("#conflict-panel");
const conflictTitle = document.querySelector<HTMLHeadingElement>("#conflict-title");
const conflictSummary = document.querySelector<HTMLElement>("#conflict-summary");
const conflictTargetLabel = document.querySelector<HTMLElement>("#conflict-target-label");
const conflictTargetPath = document.querySelector<HTMLElement>("#conflict-target-path");
const databasePathRow = document.querySelector<HTMLElement>("#database-path-row");
const databasePath = document.querySelector<HTMLElement>("#database-path");
const storageKind = document.querySelector<HTMLElement>("#storage-kind");
const ownerDetails = document.querySelector<HTMLElement>("#owner-details");
const ownerPidRow = document.querySelector<HTMLElement>("#owner-pid-row");
const ownerPid = document.querySelector<HTMLElement>("#owner-pid");
const ownerExecutableRow = document.querySelector<HTMLElement>("#owner-executable-row");
const ownerExecutable = document.querySelector<HTMLElement>("#owner-executable");
const ownerStartedRow = document.querySelector<HTMLElement>("#owner-started-row");
const ownerStarted = document.querySelector<HTMLElement>("#owner-started");
const diagnosticDetails = document.querySelector<HTMLDetailsElement>("#diagnostic-details");
const diagnosticSummary = document.querySelector<HTMLElement>("#diagnostic-summary");
const diagnosticOutput = document.querySelector<HTMLElement>("#diagnostic-output");
const failurePanel = document.querySelector<HTMLElement>("#failure-panel");
const failureTitle = document.querySelector<HTMLHeadingElement>("#failure-title");
const failureDetail = document.querySelector<HTMLElement>("#failure-detail");
const failureHome = document.querySelector<HTMLElement>("#failure-home");
const failureHomePath = document.querySelector<HTMLElement>("#failure-home-path");
const temporaryButton = document.querySelector<HTMLButtonElement>("#temporary-instance-button");
const actionFeedback = document.querySelector<HTMLElement>("#temporary-action-feedback");
const startupDragRegion = document.querySelector<HTMLElement>("[data-startup-drag-region]");

if (
  "__TAURI_INTERNALS__" in window &&
  /Macintosh|Mac OS X/i.test(navigator.userAgent)
) {
  startupDragRegion?.setAttribute("data-tauri-drag-region", "");
}

function text(key: StartupCopyKey, values?: Record<string, string | number>) {
  return getStartupText(key, values);
}

function setText(element: Element | null, value: string) {
  if (element) element.textContent = value;
}

function setLabel(element: Element | null, key: StartupCopyKey) {
  setText(element, text(key));
}

function setRow(element: HTMLElement | null, value: string | number | undefined) {
  if (!element) return;
  element.hidden = value === undefined || value === "";
}

function translatedStorageKind(value: StartupConflict["storage_kind"] | undefined) {
  const keyByStorageKind: Record<StartupConflict["storage_kind"], StartupCopyKey> = {
    sqlite_in_home: "sqliteInHome",
    sqlite_external: "externalSqlite",
    postgres: "postgresStorage",
  };
  return value ? text(keyByStorageKind[value]) : text("unknownStorage");
}

function displayConflict(conflict: StartupConflict | undefined) {
  if (!conflict) return;

  setText(conflictTitle, text("conflictTitle"));
  setText(
    conflictSummary,
    text(conflict.target_kind === "database" ? "conflictDatabaseDetail" : "conflictHomeDetail"),
  );
  setText(
    conflictTargetLabel,
    text(conflict.target_kind === "database" ? "databasePathLabel" : "dataFolderLabel"),
  );
  setText(conflictTargetPath, conflict.target_path);
  setText(document.querySelector("#database-path-label"), text("databasePathLabel"));
  setText(storageKind, translatedStorageKind(conflict.storage_kind));
  if (conflict.database_path) {
    setText(databasePath, conflict.database_path);
    if (databasePathRow) databasePathRow.hidden = false;
  } else if (databasePathRow) {
    databasePathRow.hidden = true;
  }

  const owner = conflict.owner;
  setRow(ownerPidRow, owner?.pid);
  setText(ownerPid, owner?.pid === undefined ? "" : String(owner.pid));
  setRow(ownerExecutableRow, owner?.executable);
  setText(ownerExecutable, owner?.executable ?? "");
  setRow(ownerStartedRow, owner?.started_at);
  setText(ownerStarted, owner?.started_at ?? "");
  if (ownerDetails)
    ownerDetails.hidden = !owner || (!owner.pid && !owner.executable && !owner.started_at);
}

function applyStatus(status: DesktopStatus) {
  const isConflict = status.kind === "conflict";
  const isFailure = status.kind === "failure";
  const isLoading = status.kind === "loading" || status.kind === "temporary";

  if (shell) {
    shell.setAttribute("aria-busy", isLoading ? "true" : "false");
    shell.classList.toggle("is-failed", isFailure);
    shell.classList.toggle("is-conflict", isConflict);
    shell.classList.toggle("is-temporary", status.kind === "temporary");
  }
  if (loadingState) loadingState.hidden = !isLoading;
  if (conflictPanel) conflictPanel.hidden = !isConflict;
  if (failurePanel) failurePanel.hidden = !isFailure;

  if (isLoading) {
    setText(loadingTitle, text(status.kind === "temporary" ? "temporaryTitle" : "loadingTitle"));
    setText(
      loadingDetail,
      text(status.kind === "temporary" ? "temporaryStarting" : "loadingDetail"),
    );
    if (loadingHome) loadingHome.hidden = status.kind !== "temporary";
    setLabel(document.querySelector("#temporary-home-label"), "temporaryHomeLabel");
    setText(loadingHomePath, status.home ?? "");
    setText(document.querySelector("#temporary-cleanup"), text("temporaryCleanup"));
    spinnerStatus?.setAttribute("aria-label", text("loadingTitle"));
  }

  if (isConflict) {
    displayConflict(status.conflict);
    const ruleKey =
      status.conflict?.target_kind === "home" &&
      status.conflict.storage_kind !== "sqlite_in_home"
        ? "singleDataFolderRule"
        : "singleDatabaseRule";
    setText(document.querySelector("#conflict-rule"), text(ruleKey));
    setText(document.querySelector("#temporary-heading"), text("temporaryHeading"));
    setText(document.querySelector("#temporary-warning"), text("temporaryWarning"));
    setText(temporaryButton, text("temporaryAction"));
    setText(actionFeedback, "");
    setLabel(document.querySelector("#owner-label"), "ownerLabel");
    setLabel(document.querySelector("#pid-label"), "pidLabel");
    setLabel(document.querySelector("#executable-label"), "executableLabel");
    setLabel(document.querySelector("#started-label"), "startedLabel");
    setLabel(document.querySelector("#storage-label"), "storageKindLabel");
    setLabel(document.querySelector("#temporary-home-label"), "temporaryHomeLabel");
    setLabel(document.querySelector("#diagnostic-summary"), "technicalDetails");
    if (diagnosticOutput) diagnosticOutput.textContent = status.detail ?? "";
    if (diagnosticDetails) diagnosticDetails.hidden = !status.detail;
  }

  if (isFailure) {
    setText(failureTitle, text("startupFailureTitle"));
    setText(failureDetail, status.detail ?? "");
    if (failureHome) failureHome.hidden = !status.home;
    setLabel(document.querySelector("#failure-home-label"), "temporaryHomeLabel");
    setText(failureHomePath, status.home ?? "");
  }

  document.documentElement.lang = startupLocale;
}

function localizeStaticCopy() {
  document.documentElement.lang = startupLocale;
  const main = document.querySelector<HTMLElement>("#startup-shell");
  main?.setAttribute("aria-label", text("statusRegionLabel"));
  setText(loadingTitle, text("loadingTitle"));
  setText(loadingDetail, text("loadingDetail"));
  setText(document.querySelector("#temporary-home-label"), text("temporaryHomeLabel"));
  setText(document.querySelector("#temporary-cleanup"), text("temporaryCleanup"));
  spinnerStatus?.setAttribute("aria-label", text("loadingTitle"));
  setText(document.querySelector("#conflict-rule"), text("singleDatabaseRule"));
  setText(document.querySelector("#temporary-heading"), text("temporaryHeading"));
  setText(document.querySelector("#temporary-warning"), text("temporaryWarning"));
  setText(temporaryButton, text("temporaryAction"));
  setText(failureTitle, text("startupFailureTitle"));
  setLabel(document.querySelector("#failure-home-label"), "temporaryHomeLabel");
  setLabel(document.querySelector("#owner-label"), "ownerLabel");
  setLabel(document.querySelector("#pid-label"), "pidLabel");
  setLabel(document.querySelector("#executable-label"), "executableLabel");
  setLabel(document.querySelector("#started-label"), "startedLabel");
  setLabel(document.querySelector("#storage-label"), "storageKindLabel");
  setLabel(document.querySelector("#database-path-label"), "databasePathLabel");
  setLabel(document.querySelector("#diagnostic-summary"), "technicalDetails");
}

async function startTemporaryInstance() {
  if (!temporaryButton || !actionFeedback) return;
  temporaryButton.disabled = true;
  actionFeedback.setAttribute("aria-live", "polite");
  setText(actionFeedback, text("temporaryStarting"));
  try {
    await invoke("start_temporary_test_instance");
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    actionFeedback.setAttribute("aria-live", "assertive");
    setText(actionFeedback, `${text("spawnError")} ${detail}`);
  } finally {
    temporaryButton.disabled = false;
  }
}

localizeStaticCopy();
temporaryButton?.addEventListener("click", () => void startTemporaryInstance());
diagnosticDetails?.addEventListener("toggle", () => {
  if (!diagnosticSummary) return;
  diagnosticSummary.textContent = text(
    diagnosticDetails.open ? "technicalDetailsHide" : "technicalDetails",
  );
});
window.__KANDEV_DESKTOP_SET_STATUS = applyStatus;
if (window.__KANDEV_DESKTOP_PENDING_STATUS) {
  applyStatus(window.__KANDEV_DESKTOP_PENDING_STATUS);
}
