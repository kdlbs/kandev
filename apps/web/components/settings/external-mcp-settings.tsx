"use client";

import { useMemo, useState } from "react";
import {
  IconCheck,
  IconChevronDown,
  IconClipboard,
  IconPlugConnected,
  IconCode,
  IconTools,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { getBackendConfig } from "@/lib/config";
import {
  buildAuggieCliCommand,
  buildAuggieConfig,
  buildClaudeCodeCliCommand,
  buildClaudeCodeConfig,
  buildCodexCliCommand,
  buildCodexConfig,
  buildCopilotCliConfig,
  buildCursorConfig,
  buildOpenCodeConfig,
} from "@/lib/settings/external-mcp-snippets";
import { EXTERNAL_MCP_TOOL_GROUPS, countExternalMcpTools } from "@/lib/settings/external-mcp-tools";

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
          Use this if you want to manage Kandev from coding agents that run <strong>outside</strong>{" "}
          Kandev (e.g. Claude Code, Cursor, or Codex on your host), or from{" "}
          <strong>passthrough agents</strong> running inside Kandev. <br />
          Agents launched inside Kandev in their normal mode already have the Kandev MCP wired in
          automatically, no setup needed.
        </p>
      </div>

      <ToolsPreview />

      <Separator />

      <SettingsSection
        icon={<IconPlugConnected className="h-5 w-5" />}
        title="Endpoints"
        description="Loopback-only: requests from anything other than 127.0.0.1 / ::1 are rejected. No authentication is required because the endpoint is only reachable from this machine."
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

      <SnippetsSection streamableUrl={streamableUrl} copied={copied} onCopy={handleCopy} />
    </div>
  );
}

function ToolsPreview() {
  const [open, setOpen] = useState(false);
  const total = countExternalMcpTools();
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="rounded-md border bg-muted/30">
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="flex w-full items-center gap-3 px-4 py-3 text-left cursor-pointer hover:bg-muted/50 rounded-md"
        >
          <IconTools className="h-4 w-4 text-muted-foreground shrink-0" />
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium">Available tools</p>
            <p className="text-xs text-muted-foreground">
              {total} tools across {EXTERNAL_MCP_TOOL_GROUPS.length} categories &mdash; expand to
              preview what external agents can do.
            </p>
          </div>
          <IconChevronDown
            className={`h-4 w-4 text-muted-foreground shrink-0 transition-transform ${
              open ? "rotate-180" : ""
            }`}
          />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent className="px-4 pb-4 pt-1 space-y-4">
        {EXTERNAL_MCP_TOOL_GROUPS.map((group) => (
          <div key={group.title} className="space-y-1.5">
            <div>
              <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                {group.title}
              </p>
              <p className="text-xs text-muted-foreground">{group.description}</p>
            </div>
            <ul className="space-y-1">
              {group.tools.map((tool) => (
                <li key={tool.name} className="flex gap-2 text-xs">
                  <code className="font-mono text-foreground shrink-0">{tool.name}</code>
                  <span className="text-muted-foreground">&mdash; {tool.description}</span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </CollapsibleContent>
    </Collapsible>
  );
}

function SnippetsSection({
  streamableUrl,
  copied,
  onCopy,
}: {
  streamableUrl: string;
  copied: string | null;
  onCopy: (text: string) => void;
}) {
  return (
    <SettingsSection
      icon={<IconCode className="h-5 w-5" />}
      title="Configuration snippets"
      description="Paste these into your agent's global MCP configuration."
    >
      <SnippetCard
        title="Claude Code"
        subtitle="~/.claude.json — or run the CLI command below"
        snippet={buildClaudeCodeConfig(streamableUrl)}
        copied={copied}
        onCopy={onCopy}
        extraSnippet={buildClaudeCodeCliCommand(streamableUrl)}
        extraSnippetLabel="One-liner (writes to ~/.claude.json)"
      />
      <SnippetCard
        title="Cursor"
        subtitle="~/.cursor/mcp.json"
        snippet={buildCursorConfig(streamableUrl)}
        copied={copied}
        onCopy={onCopy}
      />
      <SnippetCard
        title="Codex"
        subtitle="~/.codex/config.toml — or run the CLI command below"
        snippet={buildCodexConfig(streamableUrl)}
        copied={copied}
        onCopy={onCopy}
        extraSnippet={buildCodexCliCommand(streamableUrl)}
        extraSnippetLabel="One-liner (writes to ~/.codex/config.toml)"
      />
      <SnippetCard
        title="Auggie CLI"
        subtitle="~/.augment/settings.json — or run the CLI command below"
        snippet={buildAuggieConfig(streamableUrl)}
        copied={copied}
        onCopy={onCopy}
        extraSnippet={buildAuggieCliCommand(streamableUrl)}
        extraSnippetLabel="One-liner (writes to settings.json)"
      />
      <SnippetCard
        title="OpenCode"
        subtitle="opencode.json (project) or ~/.config/opencode/opencode.json (global)"
        snippet={buildOpenCodeConfig(streamableUrl)}
        copied={copied}
        onCopy={onCopy}
      />
      <SnippetCard
        title="GitHub Copilot CLI"
        subtitle="~/.copilot/mcp-config.json"
        snippet={buildCopilotCliConfig(streamableUrl)}
        copied={copied}
        onCopy={onCopy}
      />
    </SettingsSection>
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
  extraSnippet,
  extraSnippetLabel,
}: {
  title: string;
  subtitle: string;
  snippet: string;
  copied: string | null;
  onCopy: (text: string) => void;
  extraSnippet?: string;
  extraSnippetLabel?: string;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        <p className="text-xs text-muted-foreground font-mono">{subtitle}</p>
      </CardHeader>
      <CardContent className="space-y-3">
        <SnippetBlock snippet={snippet} copied={copied} onCopy={onCopy} />
        {extraSnippet ? (
          <div className="space-y-1.5">
            {extraSnippetLabel ? (
              <p className="text-xs text-muted-foreground">{extraSnippetLabel}</p>
            ) : null}
            <SnippetBlock snippet={extraSnippet} copied={copied} onCopy={onCopy} />
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

function SnippetBlock({
  snippet,
  copied,
  onCopy,
}: {
  snippet: string;
  copied: string | null;
  onCopy: (text: string) => void;
}) {
  const isCopied = copied === snippet;
  return (
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
  );
}
