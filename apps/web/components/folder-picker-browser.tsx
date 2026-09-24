"use client";

import { Fragment, useState, type ReactNode } from "react";
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
import type { DirectoryListing } from "@/lib/api/domains/fs-api";
import { useTranslation } from "react-i18next";

export type FolderPickerSurfaceProps = {
  autoOpen: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: string;
  pathInput: string;
  onPathInputChange: (value: string) => void;
  listing: DirectoryListing | null;
  loading: boolean;
  error: string | null;
  load: (path: string) => Promise<void>;
  allowScratch: boolean;
  hasValue: boolean;
  trigger: ReactNode;
  onChange: (path: string) => void;
};

export function FolderPickerSurface(props: FolderPickerSurfaceProps) {
  return props.autoOpen ? <FolderPickerInline {...props} /> : <FolderPickerPopover {...props} />;
}

function FolderPickerInline({
  pathInput,
  onPathInputChange,
  load,
  loading,
  listing,
  error,
  allowScratch,
  onChange,
}: FolderPickerSurfaceProps) {
  return (
    <div className="flex min-w-0 flex-col" data-testid="folder-picker-inline">
      <FolderPathInput
        value={pathInput}
        onChange={onPathInputChange}
        onNavigate={() => void load(pathInput.trim())}
        disabled={loading}
      />
      <DirectoryBrowserBody
        listing={listing}
        loading={loading}
        error={error}
        onNavigate={(path) => {
          onPathInputChange(path);
          void load(path);
        }}
      />
      <Footer
        choosable={listing?.choosable === true}
        allowScratch={allowScratch}
        onUseScratch={() => onChange("")}
        onChoose={() => {
          if (listing) onChange(listing.path);
        }}
      />
    </div>
  );
}

function FolderPickerPopover({
  open,
  onOpenChange,
  value,
  pathInput,
  onPathInputChange,
  listing,
  loading,
  error,
  load,
  allowScratch,
  hasValue,
  trigger,
  onChange,
}: FolderPickerSurfaceProps) {
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
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
        <FolderPathInput
          value={pathInput}
          onChange={onPathInputChange}
          onNavigate={() => void load(pathInput.trim())}
          disabled={loading}
        />
        <DirectoryBrowserBody
          listing={listing}
          loading={loading}
          error={error}
          onNavigate={(path) => void load(path)}
        />
        <Footer
          choosable={listing?.choosable === true}
          allowScratch={allowScratch}
          onUseScratch={() => {
            onChange("");
            onOpenChange(false);
          }}
          onChoose={() => {
            if (!listing) return;
            onChange(listing.path);
            onOpenChange(false);
          }}
        />
      </PopoverContent>
    </Popover>
  );
}

export function FolderPathInput({
  value,
  onChange,
  onNavigate,
  disabled,
}: {
  value: string;
  onChange: (value: string) => void;
  onNavigate: () => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2 border-b border-border px-2 py-2">
      <Input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            event.preventDefault();
            onNavigate();
          }
        }}
        placeholder={t("task:folderPathPlaceholder")}
        aria-label={t("task:folderPath")}
        data-testid="folder-picker-path-input"
        className="h-9 min-w-0 flex-1 font-mono text-xs"
      />
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={onNavigate}
        disabled={disabled || !value.trim()}
        data-testid="folder-picker-path-go"
      >
        {t("common:go")}
      </Button>
    </div>
  );
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

export function Footer({
  choosable,
  allowScratch,
  onUseScratch,
  onChoose,
}: {
  choosable: boolean;
  allowScratch: boolean;
  onUseScratch: () => void;
  onChoose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-2 border-t border-border bg-muted/20 px-2 py-1.5">
      {allowScratch ? (
        <button
          type="button"
          onClick={onUseScratch}
          data-testid="folder-picker-use-scratch"
          className="inline-flex items-center gap-1.5 rounded px-2 py-1 text-[11px] text-muted-foreground hover:bg-accent hover:text-foreground cursor-pointer"
        >
          <IconBox className="h-3 w-3" />
          {t("common:useScratchInstead")}
        </button>
      ) : (
        <span />
      )}
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
