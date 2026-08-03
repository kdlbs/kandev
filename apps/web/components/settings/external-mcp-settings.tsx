"use client";

import { useMemo, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
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
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
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
  const { t } = useTranslation();
  const baseUrl = useMemo(() => getBackendConfig().apiBaseUrl.replace(/\/$/, ""), []);
  const streamableUrl = `${baseUrl}/mcp`;
  const sseUrl = `${baseUrl}/mcp/sse`;
  const [copied, setCopied] = useState<string | null>(null);

  function handleCopy(text: string) {
    void copyToClipboard(text).then((success) => {
      if (success) {
        setCopied(text);
        setTimeout(() => setCopied(null), 2000);
      }
    });
  }

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">{t("common:externalMcp")}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          <Trans i18nKey="settings:externalMcpIntro">
            Use this if you want to manage Kandev from coding agents that run{" "}
            <strong>outside</strong> Kandev (e.g. Claude Code, Cursor, or Codex on your host), or
            from <strong>passthrough agents</strong> running inside Kandev.
          </Trans>{" "}
          <br />
          {t("settings:externalMcpIntroPassthrough")}
        </p>
      </div>

      <ToolsPreview />

      <Separator />

      <SettingsSection
        icon={<IconPlugConnected className="h-5 w-5" />}
        title={t("settings:externalMcpEndpoints")}
        description={t("settings:externalMcpEndpointsDescription")}
      >
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("settings:externalMcpStreamableHttp")}</CardTitle>
          </CardHeader>
          <CardContent>
            <UrlRow url={streamableUrl} copied={copied} onCopy={handleCopy} />
            <p className="text-xs text-muted-foreground mt-2">
              {t("settings:externalMcpStreamableHttpHint")}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("settings:externalMcpSse")}</CardTitle>
          </CardHeader>
          <CardContent>
            <UrlRow url={sseUrl} copied={copied} onCopy={handleCopy} />
            <p className="text-xs text-muted-foreground mt-2">{t("settings:externalMcpSseHint")}</p>
          </CardContent>
        </Card>
      </SettingsSection>

      <Separator />

      <SnippetsSection streamableUrl={streamableUrl} copied={copied} onCopy={handleCopy} />
    </div>
  );
}

function ToolsPreview() {
  const { t } = useTranslation();
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
            <p className="text-sm font-medium">{t("settings:externalMcpAvailableTools")}</p>
            <p className="text-xs text-muted-foreground">
              {t("settings:externalMcpToolsSummary", {
                tools: t("settings:externalMcpToolCount", { count: total }),
                categories: t("settings:externalMcpCategoryCount", {
                  count: EXTERNAL_MCP_TOOL_GROUPS.length,
                }),
              })}
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
          <div key={group.titleKey} className="space-y-1.5">
            <div>
              <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                {t(group.titleKey)}
              </p>
              <p className="text-xs text-muted-foreground">{t(group.descriptionKey)}</p>
            </div>
            <ul className="space-y-1">
              {group.tools.map((tool) => (
                <li key={tool.name} className="flex gap-2 text-xs">
                  <code className="font-mono text-foreground shrink-0">{tool.name}</code>
                  <span className="text-muted-foreground">
                    &mdash; {t(tool.descriptionKey, tool.descriptionValues)}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </CollapsibleContent>
    </Collapsible>
  );
}

/**
 * One card per external agent. `title` is a product name and every `path` is a
 * config filename the user must find on disk, so both stay English in every
 * locale — the surrounding sentence is the only copy, and it interpolates them
 * so the pseudo-locale cannot turn them into dead pointers.
 */
const SNIPPET_CARDS: Array<{
  title: string;
  build: (streamableUrl: string) => string;
  /** Rendered through `externalMcpConfigPath*`; omitted when `subtitleKey` is set. */
  path?: string;
  /** Overrides the derived subtitle when the path sentence needs its own message. */
  subtitleKey?: string;
  subtitleValues?: Record<string, string>;
  buildCli?: (streamableUrl: string) => string;
  /** File the one-liner writes to, as the user would recognise it. */
  cliTarget?: string;
}> = [
  {
    title: "Claude Code",
    path: "~/.claude.json",
    build: buildClaudeCodeConfig,
    buildCli: buildClaudeCodeCliCommand,
    cliTarget: "~/.claude.json",
  },
  { title: "Cursor", path: "~/.cursor/mcp.json", build: buildCursorConfig },
  {
    title: "Codex",
    path: "~/.codex/config.toml",
    build: buildCodexConfig,
    buildCli: buildCodexCliCommand,
    cliTarget: "~/.codex/config.toml",
  },
  {
    title: "Auggie CLI",
    path: "~/.augment/settings.json",
    build: buildAuggieConfig,
    buildCli: buildAuggieCliCommand,
    cliTarget: "settings.json",
  },
  {
    title: "OpenCode",
    subtitleKey: "settings:externalMcpOpenCodePaths",
    subtitleValues: {
      projectPath: "opencode.json",
      globalPath: "~/.config/opencode/opencode.json",
    },
    build: buildOpenCodeConfig,
  },
  { title: "GitHub Copilot CLI", path: "~/.copilot/mcp-config.json", build: buildCopilotCliConfig },
];

function SnippetsSection({
  streamableUrl,
  copied,
  onCopy,
}: {
  streamableUrl: string;
  copied: string | null;
  onCopy: (text: string) => void;
}) {
  const { t } = useTranslation();
  const subtitleFor = (card: (typeof SNIPPET_CARDS)[number]) => {
    if (card.subtitleKey) return t(card.subtitleKey, card.subtitleValues);
    // A bare path is the whole subtitle here, and a path is not copy — render it
    // directly rather than through a catalog message whose entire value is
    // `{{path}}`. Such a message says nothing a translator can act on, and it
    // is a shape worth avoiding generally: anything that compiles a message
    // into a match pattern has to special-case it, since a placeholder-only
    // message otherwise matches every string.
    if (!card.buildCli) return card.path ?? "";
    return t("settings:externalMcpConfigPathOrCli", { path: card.path });
  };

  return (
    <SettingsSection
      icon={<IconCode className="h-5 w-5" />}
      title={t("settings:externalMcpSnippets")}
      description={t("settings:externalMcpSnippetsDescription")}
    >
      {SNIPPET_CARDS.map((card) => (
        <SnippetCard
          key={card.title}
          title={card.title}
          subtitle={subtitleFor(card)}
          snippet={card.build(streamableUrl)}
          copied={copied}
          onCopy={onCopy}
          extraSnippet={card.buildCli?.(streamableUrl)}
          extraSnippetLabel={
            card.cliTarget ? t("settings:externalMcpOneLiner", { path: card.cliTarget }) : undefined
          }
        />
      ))}
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
  const { t } = useTranslation();
  const isCopied = copied === url;
  return (
    <div className="flex items-center gap-1 rounded-md bg-muted px-2 py-1.5 font-mono text-xs">
      <code className="flex-1 truncate">{url}</code>
      <Button
        variant="ghost"
        size="sm"
        className="h-7 w-7 p-0 cursor-pointer shrink-0"
        aria-label={isCopied ? t("settings:externalMcpCopied") : t("settings:externalMcpCopyUrl")}
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
  const { t } = useTranslation();
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
        title={t("settings:externalMcpCopyToClipboard")}
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
