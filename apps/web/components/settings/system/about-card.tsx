"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { IconInfoCircle, IconExternalLink } from "@tabler/icons-react";
import { useSystemInfo } from "@/hooks/domains/system/use-system-info";

function Row({ label, value, testid }: { label: string; value: string; testid: string }) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-1.5 border-b last:border-b-0">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="text-sm font-mono break-all text-right" data-testid={testid}>
        {value || "-"}
      </span>
    </div>
  );
}

export function AboutCard() {
  const { info, isLoading } = useSystemInfo();

  if (!info && isLoading) {
    return (
      <Card data-testid="system-about-card">
        <CardContent className="py-6 flex items-center gap-2 text-sm text-muted-foreground">
          <Spinner className="size-4" /> Loading...
        </CardContent>
      </Card>
    );
  }

  return (
    <Card data-testid="system-about-card">
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <IconInfoCircle className="h-4 w-4" /> About kandev
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="rounded-md border px-3 py-2">
          <Row label="Version" value={info?.version ?? "-"} testid="system-about-version" />
          <Row label="Commit" value={info?.commit ?? "-"} testid="system-about-commit" />
          <Row
            label="Build time"
            value={info?.build_time ?? "-"}
            testid="system-about-build-time"
          />
          <Row
            label="Go version"
            value={info?.go_version ?? "-"}
            testid="system-about-go-version"
          />
          <Row label="OS" value={info?.os ?? "-"} testid="system-about-os" />
          <Row label="Arch" value={info?.arch ?? "-"} testid="system-about-arch" />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button asChild variant="outline" size="sm" className="cursor-pointer">
            <a
              href="https://github.com/kdlbs/kandev"
              target="_blank"
              rel="noreferrer"
              data-testid="system-about-github-link"
            >
              GitHub <IconExternalLink className="h-3.5 w-3.5 ml-1" />
            </a>
          </Button>
          <Button asChild variant="outline" size="sm" className="cursor-pointer">
            <a
              href="https://kandev.ai/docs"
              target="_blank"
              rel="noreferrer"
              data-testid="system-about-docs-link"
            >
              Documentation <IconExternalLink className="h-3.5 w-3.5 ml-1" />
            </a>
          </Button>
          <Button asChild variant="outline" size="sm" className="cursor-pointer">
            <a
              href="https://github.com/kdlbs/kandev/issues/new"
              target="_blank"
              rel="noreferrer"
              data-testid="system-about-issue-link"
            >
              Report an issue <IconExternalLink className="h-3.5 w-3.5 ml-1" />
            </a>
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
