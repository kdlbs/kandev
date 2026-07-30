"use client";

import Link from "@/components/routing/app-link";
import { IconExternalLink } from "@tabler/icons-react";

/**
 * Title link for GitLab issue/MR rows.
 *
 * Must stay block-level (`flex`, not `inline-flex`): an inline-flex link sizes
 * itself to its content, so a long title escapes the row's `min-w-0 flex-1`
 * column and draws over the row actions. Truncation belongs on the text span —
 * `overflow-hidden` zeroes its automatic minimum size so it can shrink.
 */
export function RowTitleLink({ href, title }: { href: string; title: string }) {
  return (
    <Link
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      title={title}
      className="flex min-w-0 items-center gap-1.5 text-sm font-semibold hover:underline cursor-pointer"
    >
      <span className="truncate">{title}</span>
      <IconExternalLink className="h-3 w-3 shrink-0 text-muted-foreground" />
    </Link>
  );
}
