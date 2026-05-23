"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { Badge } from "@kandev/ui/badge";
import { IconDownload, IconFileText, IconRefresh } from "@tabler/icons-react";
import { useLogFiles } from "@/hooks/domains/system/use-log-files";
import { useLogTail } from "@/hooks/domains/system/use-log-tail";
import { buildLogDownloadUrl } from "@/lib/api/domains/system-api";
import { formatBytes } from "@/lib/utils/format-bytes";

function formatTimestamp(iso: string): string {
  if (!iso) return "-";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function TailContent({
  tail,
  loading,
  inMemoryOnly,
}: {
  tail: string[];
  loading: boolean;
  inMemoryOnly: boolean;
}) {
  if (loading && tail.length === 0) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Spinner className="size-4" /> Loading log...
      </div>
    );
  }
  if (tail.length === 0) {
    return (
      <p className="text-sm text-muted-foreground" data-testid="system-log-tail-empty">
        No recent log activity captured yet. Logs will appear as Kandev does work.
      </p>
    );
  }
  return (
    <div className="space-y-2">
      {inMemoryOnly && (
        <p className="text-xs text-muted-foreground" data-testid="system-log-tail-source">
          Showing the in-memory log buffer (last ~2000 entries). Kandev is currently logging to the
          terminal, not to a file - file rotation is disabled. Set <code>logging.outputPath</code>{" "}
          in <code>config.yaml</code> to a file path to enable downloadable log files.
        </p>
      )}
      <pre
        className="max-h-[28rem] overflow-auto rounded-md border bg-muted/30 p-3 text-[11px] leading-relaxed font-mono whitespace-pre"
        data-testid="system-log-tail-content"
      >
        {tail.join("\n")}
      </pre>
    </div>
  );
}

function TailHeader({
  current,
  pending,
  onRefresh,
}: {
  current: ReturnType<typeof useLogFiles>["files"][number] | undefined;
  pending: boolean;
  onRefresh: () => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <Button
        variant="outline"
        size="sm"
        disabled={pending}
        onClick={onRefresh}
        className="cursor-pointer"
        data-testid="system-log-tail-refresh"
      >
        <IconRefresh className="h-3.5 w-3.5 mr-1" /> Refresh
      </Button>
      {current && (
        <Button
          asChild
          variant="outline"
          size="sm"
          className="cursor-pointer"
          data-testid="system-log-current-download"
        >
          <a href={buildLogDownloadUrl(current.name)} download>
            <IconDownload className="h-3.5 w-3.5 mr-1" />
            Download current
          </a>
        </Button>
      )}
    </div>
  );
}

export function LogViewer() {
  const { files, isLoading: filesLoading } = useLogFiles();
  const { tail, isLoading: tailLoading, reload: reloadTail } = useLogTail(1000);
  const [pending, setPending] = useState(false);

  const onRefresh = async () => {
    setPending(true);
    try {
      await reloadTail();
    } finally {
      setPending(false);
    }
  };

  const current = files.find((f) => f.current);
  const inMemoryOnly = !filesLoading && files.length === 0;

  return (
    <div className="space-y-6">
      <Card data-testid="system-log-tail-card">
        <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
          <CardTitle className="text-base flex items-center gap-2">
            <IconFileText className="h-4 w-4" /> Recent log output
          </CardTitle>
          <TailHeader
            current={current}
            pending={pending || tailLoading}
            onRefresh={() => void onRefresh()}
          />
        </CardHeader>
        <CardContent>
          <TailContent tail={tail} loading={tailLoading} inMemoryOnly={inMemoryOnly} />
        </CardContent>
      </Card>

      <Card data-testid="system-log-files-card">
        <CardHeader>
          <CardTitle className="text-base">Log files</CardTitle>
        </CardHeader>
        <CardContent>
          {!files.length && filesLoading && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Spinner className="size-4" /> Loading files...
            </div>
          )}
          {files.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Kind</TableHead>
                  <TableHead className="text-right">Size</TableHead>
                  <TableHead>Modified</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {files.map((f) => (
                  <TableRow key={f.name} data-testid="system-log-file-row" data-name={f.name}>
                    <TableCell className="font-mono text-xs break-all">{f.name}</TableCell>
                    <TableCell>
                      <Badge variant={f.current ? "default" : "secondary"} className="text-[10px]">
                        {f.current ? "current" : "rotated"}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-xs text-right">{formatBytes(f.size)}</TableCell>
                    <TableCell className="text-xs">{formatTimestamp(f.mtime)}</TableCell>
                    <TableCell className="text-right">
                      <Button
                        asChild
                        size="sm"
                        variant="ghost"
                        className="cursor-pointer"
                        data-testid="system-log-download"
                      >
                        <a href={buildLogDownloadUrl(f.name)} download>
                          <IconDownload className="h-3.5 w-3.5" />
                        </a>
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          {!filesLoading && files.length === 0 && (
            <p className="text-sm text-muted-foreground" data-testid="system-log-files-empty">
              No log files found. Kandev may be logging to stdout (no file rotation configured).
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
