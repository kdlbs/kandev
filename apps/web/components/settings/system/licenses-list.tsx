"use client";

import { useMemo, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  IconScale,
  IconChevronDown,
  IconChevronRight,
  IconExternalLink,
} from "@tabler/icons-react";
import type { LicenseEntry } from "@/lib/types/system";

type Props = {
  entries: LicenseEntry[];
};

function LicenseRow({ entry }: { entry: LicenseEntry }) {
  const [expanded, setExpanded] = useState(false);
  return (
    <div
      className="border-b last:border-b-0 py-2"
      data-testid="system-license-row"
      data-name={entry.name}
      data-ecosystem={entry.ecosystem}
    >
      <div className="flex items-start gap-2">
        <Button
          variant="ghost"
          size="sm"
          className="cursor-pointer px-1 h-auto py-1"
          onClick={() => setExpanded((v) => !v)}
          data-testid="system-license-toggle"
          disabled={!entry.license_text}
        >
          {expanded ? (
            <IconChevronDown className="h-3.5 w-3.5" />
          ) : (
            <IconChevronRight className="h-3.5 w-3.5" />
          )}
        </Button>
        <div className="flex-1 min-w-0">
          <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
            <span className="font-mono text-sm break-all" data-testid="system-license-name">
              {entry.name}
            </span>
            <span className="text-xs text-muted-foreground" data-testid="system-license-version">
              {entry.version}
            </span>
            <Badge
              variant="secondary"
              className="text-[10px]"
              data-testid="system-license-ecosystem"
            >
              {entry.ecosystem}
            </Badge>
            <Badge variant="outline" className="text-[10px]" data-testid="system-license-type">
              {entry.license}
            </Badge>
            {entry.repository && (
              <a
                href={entry.repository}
                target="_blank"
                rel="noreferrer"
                className="text-xs text-muted-foreground hover:text-foreground inline-flex items-center gap-1 cursor-pointer"
                data-testid="system-license-repo"
              >
                source <IconExternalLink className="h-3 w-3" />
              </a>
            )}
          </div>
          {expanded && entry.license_text && (
            <pre
              className="mt-2 max-h-72 overflow-auto rounded border bg-muted/40 p-2 text-[10px] leading-snug font-mono whitespace-pre-wrap"
              data-testid="system-license-text"
            >
              {entry.license_text}
            </pre>
          )}
        </div>
      </div>
    </div>
  );
}

function filterEntries(entries: LicenseEntry[], query: string): LicenseEntry[] {
  if (!query.trim()) return entries;
  const lower = query.toLowerCase();
  return entries.filter((e) => {
    return (
      e.name.toLowerCase().includes(lower) ||
      (e.license ?? "").toLowerCase().includes(lower) ||
      (e.ecosystem ?? "").toLowerCase().includes(lower)
    );
  });
}

export function LicensesList({ entries }: Props) {
  const [query, setQuery] = useState("");
  const visible = useMemo(() => filterEntries(entries, query), [entries, query]);

  return (
    <Card data-testid="system-licenses-card">
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <IconScale className="h-4 w-4" /> Third-party licenses
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between gap-3">
          <Input
            placeholder="Filter by name, license, or ecosystem..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            data-testid="system-licenses-filter"
          />
          <p
            className="text-xs text-muted-foreground whitespace-nowrap"
            data-testid="system-licenses-count"
          >
            {visible.length} of {entries.length}
          </p>
        </div>
        <div
          className="max-h-[calc(100vh-22rem)] overflow-auto rounded-md border px-2"
          data-testid="system-licenses-list"
        >
          {visible.map((entry) => (
            <LicenseRow key={`${entry.ecosystem}-${entry.name}-${entry.version}`} entry={entry} />
          ))}
          {visible.length === 0 && (
            <p
              className="py-6 text-center text-sm text-muted-foreground"
              data-testid="system-licenses-empty"
            >
              No entries match the filter.
            </p>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
