"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Spinner } from "@kandev/ui/spinner";
import { IconExternalLink, IconRefresh } from "@tabler/icons-react";
import { useUpdates } from "@/hooks/domains/system/use-updates";

function formatChecked(iso: string | undefined): string {
  if (!iso) return "never";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

export function UpdatesCard() {
  const { updates, check } = useUpdates();
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [retryAfter, setRetryAfter] = useState<number | null>(null);

  const onCheck = async () => {
    setChecking(true);
    setError(null);
    setRetryAfter(null);
    try {
      await check();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Update check failed";
      setError(message);
      const match = /retry.*?(\d+)/i.exec(message);
      if (match) {
        setRetryAfter(Number(match[1]));
      }
    } finally {
      setChecking(false);
    }
  };

  const current = updates?.current ?? "-";
  const latest = updates?.latest ?? "-";
  const available = updates?.update_available ?? false;
  const checkedAt = updates?.latest_checked_at;
  const url = updates?.latest_url;

  return (
    <Card data-testid="system-updates-card">
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <IconRefresh className="h-4 w-4" />
          Updates
          {available && (
            <Badge variant="default" className="text-[10px]" data-testid="system-updates-badge">
              Update available
            </Badge>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-2 gap-3 text-sm">
          <div>
            <div className="text-xs text-muted-foreground">Current version</div>
            <div className="font-mono text-sm" data-testid="system-updates-current">
              {current}
            </div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">Latest release</div>
            <div className="font-mono text-sm" data-testid="system-updates-latest">
              {latest}
            </div>
          </div>
        </div>

        <div className="text-xs text-muted-foreground" data-testid="system-updates-checked-at">
          Last checked {formatChecked(checkedAt)}
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={checking}
            onClick={() => void onCheck()}
            className="cursor-pointer"
            data-testid="system-updates-check"
          >
            {checking ? (
              <Spinner className="size-3.5 mr-1" />
            ) : (
              <IconRefresh className="h-3.5 w-3.5 mr-1" />
            )}
            Check now
          </Button>
          {url && (
            <Button
              asChild
              variant="ghost"
              size="sm"
              className="cursor-pointer"
              data-testid="system-updates-release-link"
            >
              <a href={url} target="_blank" rel="noreferrer">
                Release notes
                <IconExternalLink className="h-3.5 w-3.5 ml-1" />
              </a>
            </Button>
          )}
        </div>

        {error && (
          <p className="text-xs text-destructive" data-testid="system-updates-error">
            {retryAfter ? `Already checked. Try again in ${retryAfter}s.` : error}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
