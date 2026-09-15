"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { IconBox, IconFolder } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { listDirectory, type DirectoryListing } from "@/lib/api/domains/fs-api";
import {
  isTauriWebview,
  nativeFolderPicker,
  NativeFolderPickerUnavailableError,
} from "@/lib/desktop/folder-picker";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { FolderPickerSurface } from "@/components/folder-picker-browser";

export { DirectoryBrowserBody } from "@/components/folder-picker-browser";

type FolderPickerProps = {
  /** Currently chosen absolute path; empty when nothing is picked. */
  value: string;
  /** Called with the absolute path when the user chooses a folder. */
  onChange: (path: string) => void;
  placeholder?: string;
  /** Opens the browser as soon as the picker mounts inside a source view. */
  autoOpen?: boolean;
  /** Hides the scratch action when the picker is adding a live folder row. */
  allowScratch?: boolean;
};

type NativeFolderPickerTriggerProps = {
  triggerClass: string;
  hasValue: boolean;
  triggerLabel: string;
  loading: boolean;
  error: string | null;
  onChoose: () => void;
};

function NativeFolderPickerTrigger({
  triggerClass,
  hasValue,
  triggerLabel,
  loading,
  error,
  onChoose,
}: NativeFolderPickerTriggerProps) {
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 flex-col items-start gap-1">
      <button
        type="button"
        data-testid="folder-picker-trigger"
        className={triggerClass}
        disabled={loading}
        aria-busy={loading}
        onClick={onChoose}
      >
        {hasValue ? (
          <IconFolder className="h-3.5 w-3.5 flex-shrink-0" />
        ) : (
          <IconBox className="h-3.5 w-3.5 flex-shrink-0" />
        )}
        <span className="truncate max-w-[260px]">
          {loading ? t("common:loading") : triggerLabel}
        </span>
      </button>
      {error && (
        <p
          role="alert"
          data-testid="folder-picker-native-error"
          className="text-xs text-destructive"
        >
          {error}
        </p>
      )}
    </div>
  );
}

function nativePickerErrorMessage(error: unknown, t: TFunction): string {
  if (error instanceof NativeFolderPickerUnavailableError) {
    return t("common:nativeFolderPickerUnavailable");
  }
  if (error instanceof Error && error.message) return error.message;
  return t("common:nativeFolderPickerFailed");
}

/**
 * Folder picker for repo-less tasks. Drives GET /api/v1/fs/list-dir on the
 * local kandev backend (browsers can't enumerate the host filesystem). The
 * trigger lives in the chip row and the popover handles browse + commit.
 */
export function FolderPicker({
  value,
  onChange,
  placeholder,
  autoOpen = false,
  allowScratch = true,
}: FolderPickerProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(autoOpen);
  const [pathInput, setPathInput] = useState(value);
  const [nativeLoading, setNativeLoading] = useState(false);
  const [nativeError, setNativeError] = useState<string | null>(null);
  const tauriWebview = isTauriWebview();
  const { listing, loading, error, load } = useDirectoryListing(open && !tauriWebview, value);
  const leaf = leafName(value);
  const triggerLabel = leaf || placeholder || t("common:scratchWorkspace");
  const hasValue = !!value;

  const triggerClass = cn(
    "h-7 inline-flex items-center gap-1.5 rounded-md px-2.5 text-xs cursor-pointer [@media(pointer:coarse)]:h-11",
    "border border-border/60 transition-colors",
    hasValue
      ? "bg-primary/10 text-foreground hover:bg-primary/15"
      : "bg-muted/30 text-muted-foreground hover:bg-muted/60",
  );

  const trigger = (
    <button type="button" data-testid="folder-picker-trigger" className={triggerClass}>
      {hasValue ? (
        <IconFolder className="h-3.5 w-3.5 flex-shrink-0" />
      ) : (
        <IconBox className="h-3.5 w-3.5 flex-shrink-0" />
      )}
      <span className="truncate max-w-[260px]">{triggerLabel}</span>
    </button>
  );

  const chooseNativeFolder = async () => {
    setNativeError(null);
    if (!nativeFolderPicker.isAvailable()) {
      setNativeError(t("common:nativeFolderPickerUnavailable"));
      return;
    }
    setNativeLoading(true);
    try {
      const outcome = await nativeFolderPicker.pickDirectory();
      if (outcome.status === "selected") onChange(outcome.path);
      if (outcome.status === "failed") {
        setNativeError(outcome.message || t("common:nativeFolderPickerFailed"));
      }
    } catch (error) {
      setNativeError(nativePickerErrorMessage(error, t));
    } finally {
      setNativeLoading(false);
    }
  };

  useEffect(() => {
    setPathInput(value);
  }, [value]);

  useEffect(() => {
    if (!autoOpen || !tauriWebview) return;
    void chooseNativeFolder();
    // The source view mounts once per folder-add flow. Reopening the same view
    // is controlled by its parent, so this effect intentionally runs once.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoOpen, tauriWebview]);

  if (tauriWebview) {
    return (
      <NativeFolderPickerTrigger
        triggerClass={triggerClass}
        hasValue={hasValue}
        triggerLabel={triggerLabel}
        loading={nativeLoading}
        error={nativeError}
        onChoose={() => void chooseNativeFolder()}
      />
    );
  }

  return (
    <FolderPickerSurface
      autoOpen={autoOpen}
      open={open}
      onOpenChange={setOpen}
      value={value}
      pathInput={pathInput}
      onPathInputChange={setPathInput}
      listing={listing}
      loading={loading}
      error={error}
      load={load}
      allowScratch={allowScratch}
      hasValue={hasValue}
      trigger={trigger}
      onChange={onChange}
    />
  );
}

function leafName(path: string): string {
  if (!path) return "";
  if (path === "/") return "/";
  const trimmed = path.replace(/[\\/]+$/, "");
  if (/^[A-Za-z]:$/.test(trimmed)) return `${trimmed}\\`;
  const idx = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
  return idx === -1 ? trimmed : trimmed.slice(idx + 1) || "/";
}

export function useDirectoryListing(open: boolean, value: string) {
  const { t } = useTranslation();
  const [listing, setListing] = useState<DirectoryListing | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestGeneration = useRef(0);

  const load = useCallback(
    async (path: string) => {
      const generation = ++requestGeneration.current;
      setLoading(true);
      setError(null);
      setListing(null);
      try {
        const nextListing = await listDirectory(path);
        if (generation !== requestGeneration.current) return;
        setListing(nextListing);
      } catch (err) {
        if (generation !== requestGeneration.current) return;
        setError(err instanceof Error ? err.message : t("common:failedToLoadDirectory"));
      } finally {
        if (generation === requestGeneration.current) setLoading(false);
      }
    },
    [t],
  );

  useEffect(() => {
    // Reset cached listing when the popover closes so the next open reloads
    // from `value`. Without this, a user who browsed deep into the tree,
    // closed without committing, and reopened would still see the stale
    // last-browsed directory instead of their picked folder (or the root).
    if (!open) {
      requestGeneration.current++;
      setListing(null);
      setLoading(false);
      setError(null);
      return;
    }
    void load(value || "");
    return () => {
      requestGeneration.current++;
    };
  }, [open, load, value]);

  return { listing, loading, error, load };
}
