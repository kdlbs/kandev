"use client";

import { useCallback, useMemo } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import ReactMarkdown from "react-markdown";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Badge } from "@kandev/ui/badge";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
  PaginationEllipsis,
} from "@kandev/ui/pagination";
import { IconExternalLink } from "@tabler/icons-react";
import { remarkPlugins, markdownComponents } from "@/components/shared/markdown-components";
import { getChangelog, type ChangelogEntry } from "@/lib/changelog";
import { getReleaseUrl } from "@/lib/release-notes";

const PAGE_SIZE = 10;
const PAGE_PARAM = "page";

function parsePageParam(raw: string | null, totalPages: number): number {
  if (!raw) return 1;
  const parsed = Number.parseInt(raw, 10);
  if (Number.isNaN(parsed) || parsed < 1) return 1;
  if (parsed > totalPages) return totalPages;
  return parsed;
}

function ChangelogEntryCard({ entry }: { entry: ChangelogEntry }) {
  const releaseUrl = getReleaseUrl(entry.version);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <CardTitle className="text-base flex items-center gap-2">
            <Badge variant="secondary">v{entry.version}</Badge>
            {entry.date && (
              <span className="text-sm font-normal text-muted-foreground">{entry.date}</span>
            )}
          </CardTitle>
          <a
            href={releaseUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
          >
            View on GitHub
            <IconExternalLink className="h-3 w-3" />
          </a>
        </div>
      </CardHeader>
      <CardContent>
        <div className="markdown-body text-sm">
          <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
            {entry.notes}
          </ReactMarkdown>
        </div>
      </CardContent>
    </Card>
  );
}

function buildPageNumbers(currentPage: number, totalPages: number): (number | "ellipsis")[] {
  if (totalPages <= 5) return Array.from({ length: totalPages }, (_, i) => i + 1);

  const pages: (number | "ellipsis")[] = [1];
  if (currentPage > 3) pages.push("ellipsis");

  const start = Math.max(2, currentPage - 1);
  const end = Math.min(totalPages - 1, currentPage + 1);
  for (let i = start; i <= end; i++) pages.push(i);

  if (currentPage < totalPages - 2) pages.push("ellipsis");
  pages.push(totalPages);

  return pages;
}

export function ChangelogList() {
  const changelog = getChangelog();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const totalPages = Math.max(1, Math.ceil(changelog.length / PAGE_SIZE));
  const currentPage = parsePageParam(searchParams.get(PAGE_PARAM), totalPages);

  const setPage = useCallback(
    (next: number) => {
      const clamped = Math.min(Math.max(1, next), totalPages);
      const params = new URLSearchParams(searchParams.toString());
      if (clamped === 1) {
        params.delete(PAGE_PARAM);
      } else {
        params.set(PAGE_PARAM, String(clamped));
      }
      const qs = params.toString();
      router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
    },
    [pathname, router, searchParams, totalPages],
  );

  const pageEntries = useMemo(() => {
    const start = (currentPage - 1) * PAGE_SIZE;
    return changelog.slice(start, start + PAGE_SIZE);
  }, [changelog, currentPage]);

  if (changelog.length === 0) {
    return <p className="text-sm text-muted-foreground">No changelog entries available.</p>;
  }

  const pageNumbers = buildPageNumbers(currentPage, totalPages);

  return (
    <div className="space-y-4">
      {pageEntries.map((entry) => (
        <ChangelogEntryCard key={entry.version} entry={entry} />
      ))}
      {totalPages > 1 && (
        <Pagination>
          <PaginationContent>
            <PaginationItem>
              <PaginationPrevious
                onClick={() => setPage(currentPage - 1)}
                className={currentPage === 1 ? "pointer-events-none opacity-50" : "cursor-pointer"}
              />
            </PaginationItem>
            {pageNumbers.map((page, i) =>
              page === "ellipsis" ? (
                <PaginationItem key={`ellipsis-${i}`}>
                  <PaginationEllipsis />
                </PaginationItem>
              ) : (
                <PaginationItem key={page}>
                  <PaginationLink
                    isActive={currentPage === page}
                    onClick={() => setPage(page)}
                    className="cursor-pointer"
                  >
                    {page}
                  </PaginationLink>
                </PaginationItem>
              ),
            )}
            <PaginationItem>
              <PaginationNext
                onClick={() => setPage(currentPage + 1)}
                className={
                  currentPage === totalPages ? "pointer-events-none opacity-50" : "cursor-pointer"
                }
              />
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      )}
    </div>
  );
}
