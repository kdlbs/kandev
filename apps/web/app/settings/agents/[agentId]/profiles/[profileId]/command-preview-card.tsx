"use client";

import { useEffect, useState, useMemo } from "react";
import { IconCopy, IconCheck, IconTerminal2 } from "@tabler/icons-react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Skeleton } from "@kandev/ui/skeleton";
import { previewAgentCommandAction, type CommandPreviewResponse } from "@/app/actions/agents";

type CommandPreviewCardProps = {
  agentName: string;
  model: string;
  permissionSettings: Record<string, boolean>;
  cliPassthrough: boolean;
};

function CommandPreviewLoading() {
  return (
    <div className="space-y-2">
      <Skeleton className="h-16 w-full rounded-md" />
      <Skeleton className="h-4 w-3/4" />
    </div>
  );
}

function CommandPreviewError({ error }: { error: string }) {
  return (
    <div className="rounded-md border border-destructive/50 bg-destructive/10 p-4">
      <p className="text-xs text-destructive">{error}</p>
    </div>
  );
}

function CommandPreviewEmpty() {
  return (
    <div className="rounded-md border border-muted p-4">
      <p className="text-sm text-muted-foreground">No command preview available.</p>
    </div>
  );
}

type CommandPreviewContentProps = {
  preview: CommandPreviewResponse;
  cliPassthrough: boolean;
};

function CommandPreviewContent({ preview, cliPassthrough }: CommandPreviewContentProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    if (!preview.command_string) return;

    try {
      await navigator.clipboard.writeText(preview.command_string);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      const textArea = document.createElement("textarea");
      textArea.value = preview.command_string;
      document.body.appendChild(textArea);
      textArea.select();
      document.execCommand("copy");
      document.body.removeChild(textArea);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  return (
    <>
      <div className="relative">
        <pre className="overflow-x-auto rounded-md bg-muted p-4 pr-12 font-mono text-xs">
          <code className="whitespace-pre-wrap break-all">{preview.command_string}</code>
        </pre>
        <Button
          variant="ghost"
          size="sm"
          className="absolute right-2 top-2"
          onClick={handleCopy}
          title="Copy to clipboard"
        >
          {copied ? (
            <IconCheck className="h-4 w-4 text-green-500" />
          ) : (
            <IconCopy className="h-4 w-4" />
          )}
        </Button>
      </div>

      {!cliPassthrough && (
        <p className="text-xs text-muted-foreground">
          <code className="rounded bg-muted px-1 py-0.5">{"{prompt}"}</code> will be replaced with
          your task description or follow-up message.
        </p>
      )}
    </>
  );
}

export function CommandPreviewCard({
  agentName,
  model,
  permissionSettings,
  cliPassthrough,
}: CommandPreviewCardProps) {
  const [preview, setPreview] = useState<CommandPreviewResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const settingsKey = useMemo(
    () => JSON.stringify({ model, permissionSettings, cliPassthrough }),
    [model, permissionSettings, cliPassthrough],
  );

  useEffect(() => {
    setLoading(true);
    setError(null);

    const timeoutId = setTimeout(async () => {
      try {
        const response = await previewAgentCommandAction(agentName, {
          model,
          permission_settings: permissionSettings,
          cli_passthrough: cliPassthrough,
        });
        setPreview(response);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load command preview");
        setPreview(null);
      } finally {
        setLoading(false);
      }
    }, 300);

    return () => clearTimeout(timeoutId);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- settingsKey already includes model, permissionSettings, cliPassthrough
  }, [agentName, settingsKey]);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <IconTerminal2 className="h-5 w-5" />
          <span>Command Preview</span>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-xs text-muted-foreground">
          The CLI command that will be executed based on the current settings.
        </p>

        {loading && <CommandPreviewLoading />}
        {error && <CommandPreviewError error={error} />}
        {!loading && !error && preview && (
          <CommandPreviewContent preview={preview} cliPassthrough={cliPassthrough} />
        )}
        {!loading && !error && !preview && <CommandPreviewEmpty />}
      </CardContent>
    </Card>
  );
}
