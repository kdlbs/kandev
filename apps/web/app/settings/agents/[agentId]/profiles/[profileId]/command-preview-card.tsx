"use client";

import { useEffect, useState, useMemo } from "react";
import { IconCopy, IconCheck, IconTerminal2 } from "@tabler/icons-react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Skeleton } from "@kandev/ui/skeleton";
import { previewAgentCommandAction, type CommandPreviewResponse } from "@/app/actions/agents";
import type { CLIFlag, ProfileEnvVar } from "@/lib/types/http";

type CommandPreviewCardProps = {
  agentName: string;
  model: string;
  permissionSettings: Record<string, boolean>;
  cliPassthrough: boolean;
  cliFlags: CLIFlag[];
  envVars?: ProfileEnvVar[];
  secrets?: { id: string; name: string }[];
};

/**
 * POSIX-style single-quote escaping: wrap in single quotes and escape
 * embedded single quotes via `'\''`. Used so the previewed env-var prefix
 * is copy-pasteable into a shell.
 */
function shellQuote(value: string): string {
  if (value === "") return "''";
  if (/^[A-Za-z0-9_\-./:=@]+$/.test(value)) return value;
  return `'${value.replace(/'/g, "'\\''")}'`;
}

function buildEnvPrefix(
  envVars: ProfileEnvVar[] | undefined,
  secrets: { id: string; name: string }[] | undefined,
): string {
  if (!envVars || envVars.length === 0) return "";
  const secretNameById = new Map((secrets ?? []).map((s) => [s.id, s.name]));
  const parts = envVars.map((ev) => {
    if (ev.secret_id) {
      const name = secretNameById.get(ev.secret_id) ?? "secret";
      return `${ev.key}=$${name}`;
    }
    return `${ev.key}=${shellQuote(ev.value ?? "")}`;
  });
  return `${parts.join(" ")} `;
}

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
  envPrefix: string;
};

function CommandPreviewContent({ preview, cliPassthrough, envPrefix }: CommandPreviewContentProps) {
  const [copied, setCopied] = useState(false);
  const displayCommand = `${envPrefix}${preview.command_string ?? ""}`;

  const handleCopy = async () => {
    if (!displayCommand) return;

    try {
      await navigator.clipboard.writeText(displayCommand);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      const textArea = document.createElement("textarea");
      textArea.value = displayCommand;
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
          <code className="whitespace-pre-wrap break-all">{displayCommand}</code>
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
  cliFlags,
  envVars,
  secrets,
}: CommandPreviewCardProps) {
  const envPrefix = useMemo(() => buildEnvPrefix(envVars, secrets), [envVars, secrets]);
  const [preview, setPreview] = useState<CommandPreviewResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const settingsKey = useMemo(
    () => JSON.stringify({ model, permissionSettings, cliPassthrough, cliFlags }),
    [model, permissionSettings, cliPassthrough, cliFlags],
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
          cli_flags: cliFlags,
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
    // eslint-disable-next-line react-hooks/exhaustive-deps -- settingsKey already includes model, permissionSettings, cliPassthrough, cliFlags
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
          <CommandPreviewContent
            preview={preview}
            cliPassthrough={cliPassthrough}
            envPrefix={envPrefix}
          />
        )}
        {!loading && !error && !preview && <CommandPreviewEmpty />}
      </CardContent>
    </Card>
  );
}
