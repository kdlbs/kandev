"use client";

import { useMemo, useState } from "react";
import { IconCheck, IconClipboard, IconPlugConnected, IconCode } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { getBackendConfig } from "@/lib/config";
import {
  buildClaudeCodeConfig,
  buildCodexConfig,
  buildCursorConfig,
} from "@/lib/settings/external-mcp-snippets";

export function ExternalMcpSettings() {
  const baseUrl = useMemo(() => getBackendConfig().apiBaseUrl.replace(/\/$/, ""), []);
  const streamableUrl = `${baseUrl}/mcp`;
  const sseUrl = `${baseUrl}/mcp/sse`;
  const [copied, setCopied] = useState<string | null>(null);

  function handleCopy(text: string) {
    if (typeof navigator === "undefined") return;
    navigator.clipboard
      .writeText(text)
      .then(() => {
        setCopied(text);
        setTimeout(() => setCopied(null), 2000);
      })
      .catch(() => {
        // Best-effort: clipboard may be unavailable in non-secure contexts.
      });
  }

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">External MCP</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Connect external coding agents (Claude Code, Cursor, Codex, etc.) to Kandev so they can
          read and manage your workspaces, workflows, agents, executors, and create tasks.
        </p>
      </div>

      <Separator />

      <SettingsSection
        icon={<IconPlugConnected className="h-5 w-5" />}
        title="Endpoints"
        description="Bind to localhost only. No authentication is required because the endpoint is reachable solely from your machine."
      >
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Streamable HTTP</CardTitle>
          </CardHeader>
          <CardContent>
            <UrlRow url={streamableUrl} copied={copied} onCopy={handleCopy} />
            <p className="text-xs text-muted-foreground mt-2">
              Recommended for Claude Code, Cursor, and Codex.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Server-Sent Events (SSE)</CardTitle>
          </CardHeader>
          <CardContent>
            <UrlRow url={sseUrl} copied={copied} onCopy={handleCopy} />
            <p className="text-xs text-muted-foreground mt-2">
              Compatibility transport for older MCP clients.
            </p>
          </CardContent>
        </Card>
      </SettingsSection>

      <Separator />

      <SettingsSection
        icon={<IconCode className="h-5 w-5" />}
        title="Configuration snippets"
        description="Paste these into your agent's global MCP configuration."
      >
        <SnippetCard
          title="Claude Code"
          subtitle="~/.claude.json or ~/Library/Application Support/Claude/claude_desktop_config.json"
          snippet={buildClaudeCodeConfig(streamableUrl)}
          copied={copied}
          onCopy={handleCopy}
        />
        <SnippetCard
          title="Cursor"
          subtitle="~/.cursor/mcp.json"
          snippet={buildCursorConfig(streamableUrl)}
          copied={copied}
          onCopy={handleCopy}
        />
        <SnippetCard
          title="Codex"
          subtitle="~/.codex/config.toml"
          snippet={buildCodexConfig(streamableUrl)}
          copied={copied}
          onCopy={handleCopy}
        />
      </SettingsSection>
    </div>
  );
}

function UrlRow({
  url,
  copied,
  onCopy,
}: {
  url: string;
  copied: string | null;
  onCopy: (text: string) => void;
}) {
  const isCopied = copied === url;
  return (
    <div className="flex items-center gap-1 rounded-md bg-muted px-2 py-1.5 font-mono text-xs">
      <code className="flex-1 truncate">{url}</code>
      <Button
        variant="ghost"
        size="sm"
        className="h-7 w-7 p-0 cursor-pointer shrink-0"
        aria-label={isCopied ? "Copied" : "Copy URL"}
        onClick={() => onCopy(url)}
      >
        {isCopied ? (
          <IconCheck className="h-3.5 w-3.5 text-green-500" />
        ) : (
          <IconClipboard className="h-3.5 w-3.5 text-muted-foreground" />
        )}
      </Button>
    </div>
  );
}

function SnippetCard({
  title,
  subtitle,
  snippet,
  copied,
  onCopy,
}: {
  title: string;
  subtitle: string;
  snippet: string;
  copied: string | null;
  onCopy: (text: string) => void;
}) {
  const isCopied = copied === snippet;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        <p className="text-xs text-muted-foreground font-mono">{subtitle}</p>
      </CardHeader>
      <CardContent>
        <div className="relative">
          <pre className="overflow-x-auto rounded-md bg-muted p-4 pr-12 font-mono text-xs">
            <code className="whitespace-pre-wrap break-all">{snippet}</code>
          </pre>
          <Button
            variant="ghost"
            size="sm"
            className="absolute right-2 top-2 cursor-pointer"
            onClick={() => onCopy(snippet)}
            title="Copy to clipboard"
          >
            {isCopied ? (
              <IconCheck className="h-4 w-4 text-green-500" />
            ) : (
              <IconClipboard className="h-4 w-4" />
            )}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
