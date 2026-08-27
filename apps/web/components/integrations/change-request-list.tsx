"use client";

import type { ReactNode } from "react";
import Link from "@/components/routing/app-link";
import { IconExternalLink } from "@tabler/icons-react";
import { Spinner } from "@kandev/ui/spinner";

export type ChangeRequestListProps = {
  loading: boolean;
  error: string | null;
  emptyMessage: string;
  isEmpty: boolean;
  children: ReactNode;
};

export function ChangeRequestList({
  loading,
  error,
  emptyMessage,
  isEmpty,
  children,
}: ChangeRequestListProps) {
  let content = <div className="divide-y">{children}</div>;
  if (loading) {
    content = (
      <div className="flex justify-center py-10">
        <Spinner />
      </div>
    );
  } else if (error) {
    content = <div className="py-10 text-center text-sm text-destructive">{error}</div>;
  } else if (isEmpty) {
    content = <div className="py-10 text-center text-sm text-muted-foreground">{emptyMessage}</div>;
  }
  return <div className="overflow-hidden rounded-md border">{content}</div>;
}

export type ChangeRequestRowProps = {
  stateIcon: ReactNode;
  title: string;
  href: string;
  metadata: ReactNode;
  taskIndicator?: ReactNode;
  action?: ReactNode;
  testId?: string;
  dataAttributes?: Record<string, string | number>;
};

export function ChangeRequestRow({
  stateIcon,
  title,
  href,
  metadata,
  taskIndicator,
  action,
  testId,
  dataAttributes,
}: ChangeRequestRowProps) {
  return (
    <div
      className="flex items-start gap-3 px-4 py-3 transition-colors hover:bg-muted/40"
      data-testid={testId}
      {...dataAttributes}
    >
      <span className="mt-1 shrink-0">{stateIcon}</span>
      <div className="min-w-0 flex-1">
        <Link
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          title={title}
          className="flex min-w-0 cursor-pointer items-center gap-1.5 text-sm font-semibold hover:underline"
        >
          <span className="truncate">{title}</span>
          <IconExternalLink aria-hidden="true" className="h-3 w-3 shrink-0 text-muted-foreground" />
        </Link>
        <div className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          {metadata}
          {taskIndicator}
        </div>
      </div>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  );
}
