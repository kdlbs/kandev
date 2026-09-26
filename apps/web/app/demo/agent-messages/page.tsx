"use client";

import { useEffect, useState, useCallback } from "react";
import { sessionId, taskId, type Message, type MessageType } from "@/lib/types/http";
import { MessageRenderer } from "@/components/task/chat/message-renderer";
import {
  fetchFixtureFiles,
  fetchNormalizedMessages,
  fetchNormalizedFiles,
  fetchNormalizedEventsAsMessages,
  type NormalizedFixture,
  type DiscoveredFile,
} from "@/lib/api/domains/debug-api";
import { IconChevronDown, IconChevronRight, IconRefresh } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";

type ToolFilter = "all" | "tool_edit" | "tool_read" | "tool_execute" | "tool_call";
type ViewMode = "fixtures" | "events";

const TOOL_TABS: { value: ToolFilter; labelKey: string }[] = [
  { value: "all", labelKey: "common:all" },
  { value: "tool_edit", labelKey: "common:edit" },
  { value: "tool_read", labelKey: "common:read" },
  { value: "tool_execute", labelKey: "common:execute" },
  { value: "tool_call", labelKey: "common:call" },
];

function fixtureToMessage(fixture: NormalizedFixture, index: number): Message {
  const payload = fixture.payload as Record<string, unknown>;
  const toolType = fixture.tool_type as MessageType;

  let content = "";
  if (payload.file_path) {
    content = `${fixture.tool_type}: ${payload.file_path}`;
  } else if (payload.command) {
    content = `Execute: ${payload.command}`;
  } else {
    content = fixture.tool_type;
  }

  return {
    id: `fixture-${index}`,
    session_id: sessionId("demo"),
    task_id: taskId("demo-task"),
    author_type: "agent",
    type: toolType,
    content,
    metadata: {
      ...payload,
      tool_call_id: `fixture-${index}`,
      status: "complete",
    },
    created_at: new Date().toISOString(),
  };
}

// --- Hooks ---

function useAgentMessages() {
  const { t } = useTranslation("common");
  const [viewMode, setViewMode] = useState<ViewMode>("events");
  const [fixtureFiles, setFixtureFiles] = useState<DiscoveredFile[]>([]);
  const [selectedFixtureFile, setSelectedFixtureFile] = useState<string>("");
  const [fixtures, setFixtures] = useState<NormalizedFixture[]>([]);
  const [eventFiles, setEventFiles] = useState<DiscoveredFile[]>([]);
  const [selectedEventFile, setSelectedEventFile] = useState<string>("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [toolFilter, setToolFilter] = useState<ToolFilter>("all");

  useEffect(() => {
    async function loadFiles() {
      try {
        const [fixtureFilesData, eventFilesData] = await Promise.all([
          fetchFixtureFiles(),
          fetchNormalizedFiles(),
        ]);
        setFixtureFiles(fixtureFilesData);
        setEventFiles(eventFilesData);
        if (eventFilesData.length > 0 && !selectedEventFile) {
          setSelectedEventFile(eventFilesData[0].path);
        }
        if (fixtureFilesData.length > 0 && !selectedFixtureFile) {
          setSelectedFixtureFile(fixtureFilesData[0].path);
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : t("common:demoFailedToLoadFiles"));
      } finally {
        setLoading(false);
      }
    }
    loadFiles();
  }, [selectedEventFile, selectedFixtureFile, t]);

  const loadFixtures = useCallback(async () => {
    if (!selectedFixtureFile) return;
    setLoading(true);
    setError(null);
    try {
      const data = await fetchNormalizedMessages(selectedFixtureFile);
      setFixtures(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("common:demoFailedToLoadFixtures"));
    } finally {
      setLoading(false);
    }
  }, [selectedFixtureFile, t]);

  const loadMessages = useCallback(async () => {
    if (!selectedEventFile) return;
    setLoading(true);
    setError(null);
    try {
      const data = await fetchNormalizedEventsAsMessages(selectedEventFile);
      setMessages(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("common:demoFailedToLoadMessages"));
    } finally {
      setLoading(false);
    }
  }, [selectedEventFile, t]);

  useEffect(() => {
    if (viewMode === "fixtures" && selectedFixtureFile) {
      loadFixtures();
    } else if (viewMode === "events" && selectedEventFile) {
      loadMessages();
    }
  }, [viewMode, selectedFixtureFile, selectedEventFile, loadFixtures, loadMessages]);

  const filteredFixtures = fixtures.filter(
    (f) => toolFilter === "all" || f.tool_type === toolFilter,
  );
  const currentFiles = viewMode === "fixtures" ? fixtureFiles : eventFiles;
  const selectedFile = viewMode === "fixtures" ? selectedFixtureFile : selectedEventFile;
  const setSelectedFile = viewMode === "fixtures" ? setSelectedFixtureFile : setSelectedEventFile;
  const loadContent = viewMode === "fixtures" ? loadFixtures : loadMessages;
  const itemCount = viewMode === "fixtures" ? filteredFixtures.length : messages.length;
  const totalCount = viewMode === "fixtures" ? fixtures.length : messages.length;

  return {
    viewMode,
    setViewMode,
    loading,
    error,
    toolFilter,
    setToolFilter,
    currentFiles,
    selectedFile,
    setSelectedFile,
    loadContent,
    filteredFixtures,
    messages,
    itemCount,
    totalCount,
  };
}

// --- UI Components ---

function JsonPanel({
  title,
  data,
  defaultExpanded = false,
}: {
  title: string;
  data: unknown;
  defaultExpanded?: boolean;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded);

  return (
    <div className="border rounded-md overflow-hidden bg-muted/30">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-2 w-full px-3 py-2 text-left text-sm font-medium hover:bg-muted/50 transition-colors"
      >
        {expanded ? (
          <IconChevronDown className="h-4 w-4" />
        ) : (
          <IconChevronRight className="h-4 w-4" />
        )}
        {title}
      </button>
      {expanded && (
        <pre className="px-3 py-2 text-xs overflow-x-auto bg-background/50 border-t">
          {JSON.stringify(data, null, 2)}
        </pre>
      )}
    </div>
  );
}

function FixtureCard({ fixture, index }: { fixture: NormalizedFixture; index: number }) {
  const { t } = useTranslation("common");
  const message = fixtureToMessage(fixture, index);

  return (
    <div className="border rounded-lg overflow-hidden bg-card">
      <div className="px-4 py-2 bg-muted/30 border-b flex items-center justify-between">
        <div className="flex items-center gap-3">
          <span className="text-xs font-mono px-2 py-0.5 rounded bg-primary/10 text-primary">
            {fixture.protocol}
          </span>
          <span className="text-xs font-mono px-2 py-0.5 rounded bg-secondary/50">
            {fixture.tool_type}
          </span>
          <span className="text-xs text-muted-foreground">{fixture.tool_name}</span>
        </div>
      </div>
      <div className="p-4 border-b">
        <div className="text-xs text-muted-foreground mb-2 font-medium">
          {t("common:demoRenderedOutput")}
        </div>
        <MessageRenderer comment={message} isTaskDescription={false} taskId="demo-task" />
      </div>
      <div className="p-4 space-y-2">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
          <JsonPanel title={t("common:demoRawInput")} data={fixture.input} />
          <JsonPanel title={t("common:demoNormalizedPayload")} data={fixture.payload} />
        </div>
      </div>
    </div>
  );
}

function MessageCard({ message }: { message: Message }) {
  const { t } = useTranslation("common");
  const toolName = message.metadata?.tool_name as string | undefined;
  const status = message.metadata?.status as string | undefined;

  return (
    <div className="border rounded-lg overflow-hidden bg-card">
      <div className="px-4 py-2 bg-muted/30 border-b flex items-center justify-between">
        <div className="flex items-center gap-3">
          <span className="text-xs font-mono px-2 py-0.5 rounded bg-primary/10 text-primary">
            {message.type}
          </span>
          {toolName && (
            <span className="text-xs font-mono px-2 py-0.5 rounded bg-secondary/50">
              {toolName}
            </span>
          )}
          {status && <span className="text-xs text-muted-foreground">{status}</span>}
        </div>
      </div>
      <div className="p-4 border-b">
        <div className="text-xs text-muted-foreground mb-2 font-medium">
          {t("common:demoRenderedOutput")}
        </div>
        <MessageRenderer
          comment={message}
          isTaskDescription={false}
          taskId={message.task_id || "demo-task"}
        />
      </div>
      <div className="p-4">
        <JsonPanel title={t("common:demoMessageData")} data={message} />
      </div>
    </div>
  );
}

function ViewModeTabs({
  viewMode,
  setViewMode,
}: {
  viewMode: ViewMode;
  setViewMode: (v: ViewMode) => void;
}) {
  const { t } = useTranslation("common");
  return (
    <div className="mb-6">
      <div className="flex gap-1 p-1 bg-muted/30 rounded-lg w-fit">
        {(["events", "fixtures"] as const).map((mode) => (
          <button
            key={mode}
            onClick={() => setViewMode(mode)}
            className={`px-4 py-2 text-sm rounded-md transition-colors ${
              viewMode === mode
                ? "bg-background shadow-sm font-medium"
                : "hover:bg-muted/50 text-muted-foreground"
            }`}
          >
            {t(mode === "events" ? "common:demoNormalizedEvents" : "common:demoTestFixtures")}
          </button>
        ))}
      </div>
    </div>
  );
}

function FiltersBar({
  viewMode,
  currentFiles,
  selectedFile,
  setSelectedFile,
  toolFilter,
  setToolFilter,
}: {
  viewMode: ViewMode;
  currentFiles: DiscoveredFile[];
  selectedFile: string;
  setSelectedFile: (v: string) => void;
  toolFilter: ToolFilter;
  setToolFilter: (v: ToolFilter) => void;
}) {
  const { t } = useTranslation("common");
  return (
    <div className="mb-6 space-y-4">
      <div className="flex items-center gap-2">
        <span className="text-sm font-medium text-muted-foreground">
          {t(viewMode === "events" ? "common:demoEventFile" : "common:demoFixtureFile")}
        </span>
        <select
          value={selectedFile}
          onChange={(e) => setSelectedFile(e.target.value)}
          className="px-3 py-1.5 text-sm rounded-md border bg-background hover:bg-muted/50 transition-colors min-w-[300px]"
        >
          {currentFiles.length === 0 && <option value="">{t("common:demoNoFilesFound")}</option>}
          {currentFiles.map((file) => (
            <option key={file.path} value={file.path}>
              {t("common:demoFileOption", {
                protocol: file.protocol,
                agent: file.agent || t("common:unknown"),
                count: file.message_count,
              })}
            </option>
          ))}
        </select>
      </div>
      {viewMode === "fixtures" && (
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-muted-foreground">
            {t("common:demoToolType")}
          </span>
          <div className="flex gap-1">
            {TOOL_TABS.map((tab) => (
              <button
                key={tab.value}
                onClick={() => setToolFilter(tab.value)}
                className={`px-3 py-1.5 text-sm rounded-md transition-colors ${
                  toolFilter === tab.value
                    ? "bg-secondary text-secondary-foreground"
                    : "hover:bg-muted/50"
                }`}
              >
                {t(tab.labelKey)}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function ContentArea({
  viewMode,
  loading,
  error,
  itemCount,
  filteredFixtures,
  messages,
}: {
  viewMode: ViewMode;
  loading: boolean;
  error: string | null;
  itemCount: number;
  filteredFixtures: NormalizedFixture[];
  messages: Message[];
}) {
  const { t } = useTranslation("common");
  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <IconRefresh className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive">
        <div className="font-medium">
          {t("common:demoErrorLoading", {
            mode: t(viewMode === "events" ? "common:demoEvents" : "common:demoFixtures"),
          })}
        </div>
        <div className="text-sm">{error}</div>
      </div>
    );
  }

  if (itemCount === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        {t(viewMode === "events" ? "common:demoNoNormalizedEvents" : "common:demoNoFixtures")}
      </div>
    );
  }

  if (viewMode === "fixtures") {
    return (
      <div className="space-y-4">
        {filteredFixtures.map((fixture, index) => (
          <FixtureCard key={index} fixture={fixture} index={index} />
        ))}
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {messages.map((message, index) => (
        <MessageCard key={message.id || index} message={message} />
      ))}
    </div>
  );
}

// --- Page ---

export default function AgentMessagesPage() {
  const { t } = useTranslation("common");
  const data = useAgentMessages();

  return (
    <div className="min-h-screen bg-background p-8">
      <div className="max-w-5xl mx-auto">
        <div className="mb-8">
          <div className="flex items-center justify-between mb-2">
            <h1 className="text-2xl font-bold">{t("common:demoAgentMessagesQa")}</h1>
            <button
              onClick={data.loadContent}
              disabled={data.loading || !data.selectedFile}
              className="flex items-center gap-2 px-3 py-1.5 text-sm rounded-md border hover:bg-muted/50 disabled:opacity-50 transition-colors"
            >
              <IconRefresh className={`h-4 w-4 ${data.loading ? "animate-spin" : ""}`} />
              {t("refresh")}
            </button>
          </div>
          <p className="text-muted-foreground">{t("common:demoAgentMessagesDescription")}</p>
        </div>
        <ViewModeTabs viewMode={data.viewMode} setViewMode={data.setViewMode} />
        <FiltersBar
          viewMode={data.viewMode}
          currentFiles={data.currentFiles}
          selectedFile={data.selectedFile}
          setSelectedFile={data.setSelectedFile}
          toolFilter={data.toolFilter}
          setToolFilter={data.setToolFilter}
        />
        <div className="mb-6 text-sm text-muted-foreground">
          {t(
            data.viewMode === "events"
              ? "common:demoShowingMessages"
              : "common:demoShowingFixtures",
            {
              count: data.itemCount,
              total: data.totalCount,
            },
          )}
        </div>
        <ContentArea
          viewMode={data.viewMode}
          loading={data.loading}
          error={data.error}
          itemCount={data.itemCount}
          filteredFixtures={data.filteredFixtures}
          messages={data.messages}
        />
      </div>
    </div>
  );
}
