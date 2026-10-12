"use client";

import type { ReactNode } from "react";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { FileStatusIcon } from "@/components/shared/file-status-icon";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";
import type { FileChangeStatus } from "@/lib/utils/file-change-status";

export type CollapsibleFileHeaderProps = {
  filePath: string;
  repositoryName?: string;
  status?: FileChangeStatus;
  oldPath?: string;
  collapsed: boolean;
  expandLabel: string;
  collapseLabel: string;
  onToggleCollapse: () => void;
  mobileLeading?: ReactNode;
  desktopLeading?: ReactNode;
  mobileMetadata?: ReactNode;
  desktopMetadata?: ReactNode;
  mobileStats?: ReactNode;
  desktopStats?: ReactNode;
  actions?: ReactNode;
  actionsTestId?: string;
  identityTestId?: string;
  controlsId?: string;
  className?: string;
};

export function splitCollapsibleFilePath(filePath: string): {
  directory: string;
  name: string;
} {
  const lastSlash = filePath.lastIndexOf("/");
  if (lastSlash === -1) return { directory: "", name: filePath };
  return {
    directory: filePath.slice(0, lastSlash),
    name: filePath.slice(lastSlash + 1),
  };
}

function FilePath({ path }: { path: string }) {
  const { directory, name } = splitCollapsibleFilePath(path);
  return (
    <span
      className="flex min-w-0 flex-1 items-baseline overflow-hidden text-[13px] font-medium"
      title={path}
    >
      {directory && (
        <>
          <span
            aria-hidden="true"
            data-review-file-directory
            className="min-w-0 truncate text-muted-foreground [direction:rtl] [unicode-bidi:isolate]"
          >
            <bdi dir="ltr">{directory}</bdi>
          </span>
          <span aria-hidden="true" className="shrink-0 text-muted-foreground">
            /
          </span>
        </>
      )}
      <span data-review-file-name className="max-w-full shrink-0 truncate">
        {name}
      </span>
    </span>
  );
}

function MobileFileDetails({
  filePath,
  repositoryName,
  status,
  oldPath,
  metadata,
  stats,
}: {
  filePath: string;
  repositoryName?: string;
  status?: FileChangeStatus;
  oldPath?: string;
  metadata?: ReactNode;
  stats?: ReactNode;
}) {
  const { directory, name } = splitCollapsibleFilePath(filePath);
  return (
    <span
      className="flex min-w-0 flex-1 flex-col justify-center overflow-hidden text-left leading-none"
      title={repositoryName ? `${repositoryName}/${filePath}` : filePath}
    >
      {repositoryName && (
        <span
          data-testid="review-file-repository"
          className="truncate text-[10px] font-medium leading-3 text-primary"
        >
          {repositoryName}
        </span>
      )}
      <span data-review-file-name className="truncate text-[13px] font-medium leading-4">
        {name}
      </span>
      <span className="flex min-w-0 items-center gap-1 leading-4">
        {directory && (
          <span
            aria-hidden="true"
            data-review-file-directory
            className="min-w-0 flex-1 truncate text-[11px] text-muted-foreground [direction:rtl] [unicode-bidi:isolate]"
          >
            <bdi dir="ltr">{directory}</bdi>
          </span>
        )}
        {status && <FileStatusIcon status={status} oldPath={oldPath} className="size-3.5" />}
        {metadata}
        {stats}
      </span>
    </span>
  );
}

function CollapseButton({
  filePath,
  collapsed,
  expandLabel,
  collapseLabel,
  onToggleCollapse,
  controlsId,
  children,
  className,
}: {
  filePath: string;
  collapsed: boolean;
  expandLabel: string;
  collapseLabel: string;
  onToggleCollapse: () => void;
  controlsId?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <button
      type="button"
      aria-expanded={!collapsed}
      aria-controls={controlsId}
      aria-label={collapsed ? expandLabel : collapseLabel}
      data-file-path={filePath}
      onClick={onToggleCollapse}
      className={cn(
        "flex min-w-0 flex-1 cursor-pointer items-center gap-1.5 text-left hover:text-foreground",
        className,
      )}
    >
      {collapsed ? (
        <IconChevronRight className="size-3.5 shrink-0 text-muted-foreground" />
      ) : (
        <IconChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
      )}
      {children}
    </button>
  );
}

export function CollapsibleFileHeader({
  filePath,
  repositoryName,
  status,
  oldPath,
  collapsed,
  expandLabel,
  collapseLabel,
  onToggleCollapse,
  mobileLeading,
  desktopLeading,
  mobileMetadata,
  desktopMetadata,
  mobileStats,
  desktopStats,
  actions,
  actionsTestId,
  identityTestId = "collapsible-file-identity",
  controlsId,
  className,
}: CollapsibleFileHeaderProps) {
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touchSized = isMobile || isFinePointer === false;

  if (isMobile) {
    return (
      <div
        data-testid={identityTestId}
        className={cn("flex min-h-14 w-full min-w-0 items-center gap-1 px-2", className)}
      >
        {mobileLeading}
        <CollapseButton
          filePath={filePath}
          collapsed={collapsed}
          expandLabel={expandLabel}
          collapseLabel={collapseLabel}
          onToggleCollapse={onToggleCollapse}
          controlsId={controlsId}
          className={touchSized ? "min-h-11" : "min-h-7"}
        >
          <MobileFileDetails
            filePath={filePath}
            repositoryName={repositoryName}
            status={status}
            oldPath={oldPath}
            metadata={mobileMetadata}
            stats={mobileStats}
          />
        </CollapseButton>
        {actions}
      </div>
    );
  }

  return (
    <>
      <div
        data-testid={identityTestId}
        className={cn("flex min-w-0 flex-1 items-center gap-2", className)}
      >
        {desktopLeading}
        <CollapseButton
          filePath={filePath}
          collapsed={collapsed}
          expandLabel={expandLabel}
          collapseLabel={collapseLabel}
          onToggleCollapse={onToggleCollapse}
          controlsId={controlsId}
          className={touchSized ? "min-h-11" : "min-h-7"}
        >
          <FilePath path={filePath} />
        </CollapseButton>
        {desktopMetadata}
        {desktopStats}
      </div>
      <div data-testid={actionsTestId} className="flex items-center">
        {actions}
      </div>
    </>
  );
}
