"use client";

import { Fragment, useCallback, useEffect, useState } from "react";
import { IconFolder, IconBox, IconChevronRight } from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { listDirectory, type DirectoryListing } from "@/lib/api/domains/fs-api";

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
  const [open, setOpen] = useState(false);
  const { listing, loading, error, load } = useDirectoryListing(open, value);
  const leaf = leafName(value);
  const triggerLabel = leaf || placeholder || "scratch workspace";
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
        className="w-[440px] gap-0 p-0 overflow-hidden"
        align="start"
        sideOffset={4}
        data-testid="folder-picker-popover"
      >
        <Breadcrumb path={listing?.path ?? ""} onNavigate={(p) => void load(p)} />
        <Entries
          listing={listing}
          loading={loading}
          error={error}
          onDescend={(p) => void load(p)}
        />
        <Footer
          choosable={!!listing?.path}
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
  const trimmed = path.replace(/\/+$/, "");
  const idx = trimmed.lastIndexOf("/");
  return idx === -1 ? trimmed : trimmed.slice(idx + 1) || "/";
}

function useDirectoryListing(open: boolean, value: string) {
  const [listing, setListing] = useState<DirectoryListing | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (path: string) => {
    setLoading(true);
    setError(null);
    try {
      setListing(await listDirectory(path));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load directory");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    // Reset cached listing when the popover closes so the next open reloads
    // from `value`. Without this, a user who browsed deep into the tree,
    // closed without committing, and reopened would still see the stale
    // last-browsed directory instead of their picked folder (or the root).
    if (!open) {
      setListing(null);
      return;
    }
    if (listing) return;
    void load(value || "");
  }, [open, listing, load, value]);

  return { listing, loading, error, load };
}

/** Splits an absolute path into clickable breadcrumb segments. */
function pathSegments(path: string): Array<{ name: string; path: string }> {
  if (!path) return [];
  const parts = path.split("/").filter(Boolean);
  const segs: Array<{ name: string; path: string }> = [{ name: "/", path: "/" }];
  let acc = "";
  for (const part of parts) {
    acc += `/${part}`;
    segs.push({ name: part, path: acc });
  }
  return segs;
}

function Breadcrumb({ path, onNavigate }: { path: string; onNavigate: (p: string) => void }) {
  const segs = pathSegments(path);
  return (
    <div className="flex items-center gap-0.5 border-b border-border bg-muted/30 px-2 py-1.5 overflow-x-auto">
      {segs.length === 0 && (
        <span className="text-[11px] text-muted-foreground italic">Loading…</span>
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

function Entries({
  listing,
  loading,
  error,
  onDescend,
}: {
  listing: DirectoryListing | null;
  loading: boolean;
  error: string | null;
  onDescend: (path: string) => void;
}) {
  if (loading) return <EmptyRow text="Loading…" />;
  if (error) return <EmptyRow text={error} variant="error" testId="folder-picker-error" />;
  if (!listing || listing.entries.length === 0) {
    return <EmptyRow text="No folders here — pick this one or go up" />;
  }
  return (
    <div
      className="max-h-[280px] overflow-y-auto overscroll-contain py-1"
      onWheel={(e) => e.stopPropagation()}
    >
      {listing.entries.map((entry) => (
        <button
          key={entry.path}
          type="button"
          onClick={() => onDescend(entry.path)}
          className="group flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs hover:bg-accent cursor-pointer"
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
  return (
    <div className="flex items-center justify-between gap-2 border-t border-border bg-muted/20 px-2 py-1.5">
      <button
        type="button"
        onClick={onUseScratch}
        data-testid="folder-picker-use-scratch"
        className="inline-flex items-center gap-1.5 rounded px-2 py-1 text-[11px] text-muted-foreground hover:bg-accent hover:text-foreground cursor-pointer"
      >
        <IconBox className="h-3 w-3" />
        Use scratch instead
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
        Use this folder
      </button>
    </div>
  );
}
