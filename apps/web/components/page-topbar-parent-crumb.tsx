import type { ReactNode } from "react";
import Link from "@/components/routing/app-link";
import { BreadcrumbLink } from "@kandev/ui/breadcrumb";
import { DropdownMenuItem } from "@kandev/ui/dropdown-menu";
import { cn } from "@kandev/ui/lib/utils";

/** A breadcrumb between the leading navigation crumb and the page title. */
export type ParentCrumb = {
  label: string;
  href?: string;
  externalUrl?: string;
  phoneOnlyLink?: boolean;
  icon?: ReactNode;
  ariaLabel?: string;
  title?: string;
};

export function ParentCrumbLabel({ crumb }: { crumb: ParentCrumb }) {
  const body = (
    <>
      {crumb.icon && (
        <span aria-hidden="true" className="flex shrink-0 items-center">
          {crumb.icon}
        </span>
      )}
      <span
        aria-hidden={crumb.ariaLabel ? true : undefined}
        title={crumb.title ?? crumb.label}
        className="min-w-0 truncate"
      >
        {crumb.label}
      </span>
      {crumb.ariaLabel && <span className="sr-only">{crumb.ariaLabel}</span>}
    </>
  );

  if (crumb.externalUrl) {
    return (
      <BreadcrumbLink asChild>
        <a
          href={crumb.externalUrl}
          target="_blank"
          rel="noopener noreferrer"
          aria-label={crumb.ariaLabel}
          title={crumb.title ?? crumb.label}
          className="flex max-w-40 min-w-0 items-center gap-1.5 truncate cursor-pointer text-muted-foreground underline-offset-4 transition-colors hover:text-foreground hover:underline"
        >
          {body}
        </a>
      </BreadcrumbLink>
    );
  }

  if (crumb.href) {
    return (
      <>
        <BreadcrumbLink asChild>
          <Link
            href={crumb.href}
            className={cn(
              "flex max-w-40 min-w-0 items-center gap-1.5 truncate cursor-pointer text-muted-foreground underline-offset-4 transition-colors hover:text-foreground hover:underline",
              crumb.phoneOnlyLink && "md:hidden",
            )}
            aria-label={crumb.ariaLabel}
            title={crumb.title ?? crumb.label}
          >
            {body}
          </Link>
        </BreadcrumbLink>
        {crumb.phoneOnlyLink && (
          <span
            title={crumb.title ?? crumb.label}
            className="hidden max-w-40 min-w-0 cursor-default truncate text-muted-foreground/60 md:inline"
          >
            {body}
          </span>
        )}
      </>
    );
  }

  return (
    <span
      title={crumb.title ?? crumb.label}
      className="flex max-w-40 min-w-0 cursor-default items-center gap-1.5 truncate text-muted-foreground/60"
    >
      {body}
    </span>
  );
}

export function ParentCrumbMenuItem({ crumb }: { crumb: ParentCrumb }) {
  if (crumb.externalUrl) {
    return (
      <DropdownMenuItem asChild>
        <a
          href={crumb.externalUrl}
          target="_blank"
          rel="noopener noreferrer"
          aria-label={crumb.ariaLabel}
          title={crumb.title ?? crumb.label}
        >
          {crumb.icon && (
            <span aria-hidden="true" className="flex shrink-0 items-center">
              {crumb.icon}
            </span>
          )}
          <span>{crumb.label}</span>
        </a>
      </DropdownMenuItem>
    );
  }

  if (crumb.href) {
    return (
      <DropdownMenuItem asChild>
        <Link href={crumb.href}>{crumb.label}</Link>
      </DropdownMenuItem>
    );
  }

  return <DropdownMenuItem disabled>{crumb.label}</DropdownMenuItem>;
}
