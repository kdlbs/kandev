"use client";

import { Fragment, useCallback, useEffect, useRef, useState } from "react";
import {
  IconBox,
  IconCheck,
  IconChevronRight,
  IconFolder,
  IconFolderPlus,
  IconX,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { listDirectory, type DirectoryListing } from "@/lib/api/domains/fs-api";
import { useTranslation } from "react-i18next";

type FolderPickerProps = {
  /** Currently chosen absolute path; empty when nothing is picked. */
  value: string;
  /** Called with the absolute path when the user chooses a folder. */
  onChange: (path: string) => void;
  placeholder?: string;
};

/**
 * Folder picker for repo-less tasks. Drives GET /api/v1/fs/list-dir on the
 * local kandev backend (browsers can't enumerate the host filesystem). The
 * trigger lives in the chip row and the popover handles browse + commit.
 */
export function FolderPicker({ value, onChange, placeholder }: FolderPickerProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { listing, loading, error, load } = useDirectoryListing(open, value);
  const leaf = leafName(value);
  const triggerLabel = leaf || placeholder || t("common:scratchWorkspace");
  const hasValue = !!value;

  const triggerClass = cn(
    "h-7 inline-flex items-center gap-1.5 rounded-md px-2.5 text-xs cursor-pointer",
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

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        {hasValue ? (
          <Tooltip>
            <TooltipTrigger asChild>{trigger}</TooltipTrigger>
            <TooltipContent className="font-mono text-[11px]">{value}</TooltipContent>
          </Tooltip>
        ) : (
          trigger
        )}
      </PopoverTrigger>
      <PopoverContent
        className="w-[440px] max-w-[calc(100vw-2rem)] gap-0 p-0 overflow-hidden"
        align="start"
        sideOffset={4}
        data-testid="folder-picker-popover"
      >
        <DirectoryBrowserBody
          listing={listing}
          loading={loading}
          error={error}
          onNavigate={(p) => void load(p)}
        />
        <Footer
          choosable={listing?.choosable === true}
          onUseScratch={() => {
            onChange("");
            setOpen(false);
          }}
          onChoose={() => {
            if (!listing) return;
            onChange(listing.path);
            setOpen(false);
          }}
        />
      </PopoverContent>
    </Popover>
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

/** Splits an absolute path into clickable breadcrumb segments. */
function pathSegments(path: string): Array<{ name: string; path: string }> {
  if (!path) return [];
  const segs: Array<{ name: string; path: string }> = [{ name: "/", path: "/" }];

  const driveMatch = /^([A-Za-z]:)[\\/]*(.*)$/.exec(path);
  if (driveMatch) {
    const driveRoot = `${driveMatch[1]}\\`;
    segs.push({ name: driveRoot, path: driveRoot });
    appendPathSegments(segs, driveRoot, driveMatch[2], "\\");
    return segs;
  }

  const uncMatch = /^\\\\([^\\/]+)[\\/]([^\\/]+)[\\/]*(.*)$/.exec(path);
  if (uncMatch) {
    const shareRoot = `\\\\${uncMatch[1]}\\${uncMatch[2]}\\`;
    segs.push({ name: shareRoot, path: shareRoot });
    appendPathSegments(segs, shareRoot, uncMatch[3], "\\");
    return segs;
  }

  appendPathSegments(segs, "", path, "/");
  return segs;
}

function appendPathSegments(
  segs: Array<{ name: string; path: string }>,
  root: string,
  path: string,
  separator: "/" | "\\",
) {
  const parts = path.split(/[\\/]+/).filter(Boolean);
  let acc = root.replace(/[\\/]+$/, "");
  for (const part of parts) {
    acc += `${separator}${part}`;
    segs.push({ name: part, path: acc });
  }
}

function Breadcrumb({
  path,
  onNavigate,
  touchRows = false,
}: {
  path: string;
  onNavigate: (p: string) => void;
  touchRows?: boolean;
}) {
  const { t } = useTranslation();
  const segs = pathSegments(path);
  return (
    <div className="flex items-center gap-0.5 overflow-x-auto overflow-y-hidden border-b border-border bg-muted/30 px-2 py-1.5">
      {segs.length === 0 && (
        <span className="text-[11px] text-muted-foreground italic">{t("common:loading")}</span>
      )}
      {segs.map((seg, i) => {
        const last = i === segs.length - 1;
        return (
          <Fragment key={seg.path}>
            {i > 0 && (
              <IconChevronRight className="h-3 w-3 flex-shrink-0 text-muted-foreground/60" />
            )}
            <button
              type="button"
              onClick={() => !last && onNavigate(seg.path)}
              disabled={last}
              className={cn(
                "rounded px-1.5 py-0.5 text-[11px] font-mono whitespace-nowrap",
                touchRows && "min-h-12",
                last
                  ? "text-foreground cursor-default"
                  : "text-muted-foreground hover:bg-accent hover:text-foreground cursor-pointer",
              )}
            >
              {seg.name}
            </button>
          </Fragment>
        );
      })}
    </div>
  );
}

export function DirectoryBrowserBody({
  listing,
  loading,
  error,
  onNavigate,
  onCreateDirectory,
  touchRows = false,
  fillAvailableHeight = false,
}: {
  listing: DirectoryListing | null;
  loading: boolean;
  error: string | null;
  onNavigate: (path: string) => void;
  onCreateDirectory?: (name: string) => Promise<void>;
  touchRows?: boolean;
  fillAvailableHeight?: boolean;
}) {
  return (
    <div className={cn("flex min-h-0 flex-col", fillAvailableHeight && "flex-1")}>
      {onCreateDirectory ? (
        <DirectoryBrowserToolbar
          key={listing?.path}
          disabled={!listing || loading}
          onCreateDirectory={onCreateDirectory}
          touchRows={touchRows}
        />
      ) : null}
      <Breadcrumb path={listing?.path ?? ""} onNavigate={onNavigate} touchRows={touchRows} />
      <Entries
        listing={listing}
        loading={loading}
        error={error}
        onDescend={onNavigate}
        touchRows={touchRows}
        fillAvailableHeight={fillAvailableHeight}
      />
    </div>
  );
}

function DirectoryBrowserToolbar({
  disabled,
  onCreateDirectory,
  touchRows,
}: {
  disabled: boolean;
  onCreateDirectory: (name: string) => Promise<void>;
  touchRows: boolean;
}) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const createFolder = async () => {
    const trimmedName = name.trim();
    if (!trimmedName || creating) return;
    setCreating(true);
    setError(null);
    try {
      await onCreateDirectory(trimmedName);
      setEditing(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("common:failedToCreateFolder"));
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="shrink-0 border-b border-border bg-muted/10 px-3 py-2">
      {editing ? (
        <NewFolderForm
          name={name}
          error={error}
          creating={creating}
          touchRows={touchRows}
          onNameChange={(next) => {
            setName(next);
            setError(null);
          }}
          onSubmit={() => void createFolder()}
          onCancel={() => setEditing(false)}
        />
      ) : (
        <div className="flex items-center justify-between gap-3">
          <span className="text-xs font-medium text-muted-foreground">{t("common:folders")}</span>
          <Button
            type="button"
            variant="ghost"
            size={touchRows ? "lg" : "sm"}
            className={touchRows ? "min-h-11" : undefined}
            onClick={() => setEditing(true)}
            disabled={disabled}
          >
            <IconFolderPlus />
            {t("common:newFolder")}
          </Button>
        </div>
      )}
    </div>
  );
}

/** The inline "new folder" row, extracted to keep the toolbar under the
 * max-lines-per-function cap. */
function NewFolderForm({
  name,
  error,
  creating,
  touchRows,
  onNameChange,
  onSubmit,
  onCancel,
}: {
  name: string;
  error: string | null;
  creating: boolean;
  touchRows: boolean;
  onNameChange: (next: string) => void;
  onSubmit: () => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <div className="flex min-w-0 items-center gap-2">
        <IconFolderPlus className="size-4 shrink-0 text-muted-foreground" />
        <Input
          aria-label={t("common:newFolderName")}
          value={name}
          onChange={(event) => onNameChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              onSubmit();
            } else if (event.key === "Escape") {
              onCancel();
            }
          }}
          placeholder={t("common:folderName")}
          autoFocus
          className={cn("min-w-0 flex-1", touchRows && "h-10")}
        />
        <Button
          type="button"
          size={touchRows ? "icon-lg" : "icon"}
          className={touchRows ? "size-11" : undefined}
          onClick={onSubmit}
          disabled={!name.trim() || creating}
          aria-label={t("common:createFolder")}
          title={t("common:createFolder")}
        >
          <IconCheck />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size={touchRows ? "icon-lg" : "icon"}
          className={touchRows ? "size-11" : undefined}
          onClick={onCancel}
          aria-label={t("common:cancelNewFolder")}
          title={t("common:cancel")}
        >
          <IconX />
        </Button>
      </div>
      {error ? (
        <p role="alert" className="pl-6 text-xs text-destructive">
          {error}
        </p>
      ) : null}
    </div>
  );
}

function Entries({
  listing,
  loading,
  error,
  onDescend,
  touchRows,
  fillAvailableHeight,
}: {
  listing: DirectoryListing | null;
  loading: boolean;
  error: string | null;
  onDescend: (path: string) => void;
  touchRows?: boolean;
  fillAvailableHeight?: boolean;
}) {
  const { t } = useTranslation();
  if (loading) return <EmptyRow text={t("common:loading")} />;
  if (error) return <EmptyRow text={error} variant="error" testId="folder-picker-error" />;
  if (!listing || listing.entries.length === 0) {
    return <EmptyRow text={t("common:noFoldersHerePickThisOne")} />;
  }
  return (
    <div
      className={cn(
        "overflow-y-auto overscroll-contain py-1",
        fillAvailableHeight ? "min-h-0 flex-1" : "max-h-[280px]",
      )}
      onWheel={(e) => e.stopPropagation()}
    >
      {listing.entries.map((entry) => (
        <button
          key={entry.path}
          type="button"
          onClick={() => onDescend(entry.path)}
          className={cn(
            "group flex w-full items-center gap-2 px-3 text-left text-xs hover:bg-accent cursor-pointer",
            touchRows ? "min-h-12 py-2" : "py-1.5",
          )}
          data-testid="folder-picker-entry"
        >
          <IconFolder className="h-3.5 w-3.5 flex-shrink-0 text-muted-foreground group-hover:text-foreground" />
          <span className="truncate flex-1">{entry.name}</span>
          <IconChevronRight className="h-3 w-3 flex-shrink-0 text-muted-foreground/40 group-hover:text-muted-foreground" />
        </button>
      ))}
    </div>
  );
}

function EmptyRow({ text, variant, testId }: { text: string; variant?: "error"; testId?: string }) {
  return (
    <div
      className={cn(
        "py-8 text-center text-xs",
        variant === "error" ? "text-destructive" : "text-muted-foreground",
      )}
      data-testid={testId}
    >
      {text}
    </div>
  );
}

function Footer({
  choosable,
  onUseScratch,
  onChoose,
}: {
  choosable: boolean;
  onUseScratch: () => void;
  onChoose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-2 border-t border-border bg-muted/20 px-2 py-1.5">
      <button
        type="button"
        onClick={onUseScratch}
        data-testid="folder-picker-use-scratch"
        className="inline-flex items-center gap-1.5 rounded px-2 py-1 text-[11px] text-muted-foreground hover:bg-accent hover:text-foreground cursor-pointer"
      >
        <IconBox className="h-3 w-3" />
        {t("common:useScratchInstead")}
      </button>
      <button
        type="button"
        disabled={!choosable}
        onClick={onChoose}
        data-testid="folder-picker-choose"
        className={cn(
          "inline-flex items-center gap-1.5 rounded px-2.5 py-1 text-[11px] font-medium transition-colors cursor-pointer",
          "bg-primary text-primary-foreground hover:bg-primary/90",
          "disabled:opacity-40 disabled:cursor-not-allowed",
        )}
      >
        <IconFolder className="h-3 w-3" />
        {t("common:useThisFolder")}
      </button>
    </div>
  );
}
