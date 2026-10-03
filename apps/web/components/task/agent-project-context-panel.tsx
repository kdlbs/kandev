"use client";

import { IconArrowLeft, IconDeviceFloppy, IconFolder, IconFile } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAgentProjectContext } from "@/hooks/domains/agent-projects/use-agent-project-context";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type ProjectContext = ReturnType<typeof useAgentProjectContext>;

function AgentProjectContextFile({
  context,
  touchButton,
}: {
  context: ProjectContext;
  touchButton: string;
}) {
  const { t } = useTranslation();
  const file = context.file;
  if (!file) return null;
  return (
    <div className="flex h-full min-h-0 flex-col gap-2 p-2" data-testid="agent-project-context">
      <div className="flex shrink-0 items-center gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className={cn("shrink-0", touchButton)}
          onClick={context.goUp}
          aria-label={t("projects:backToContext")}
        >
          <IconArrowLeft className="h-4 w-4" />
        </Button>
        <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">{file.path}</span>
        <Button
          type="button"
          size="sm"
          className={cn("shrink-0", touchButton)}
          disabled={!context.dirty || context.busy}
          onClick={() => void context.save()}
        >
          <IconDeviceFloppy className="mr-1 h-4 w-4" />
          {t("projects:saveContext")}
        </Button>
      </div>
      {context.error && (
        <p role="alert" className="shrink-0 text-xs text-destructive">
          {context.error}
        </p>
      )}
      <textarea
        aria-label={t("projects:contextFileContent")}
        className="min-h-0 flex-1 resize-none rounded-md border bg-background p-3 font-mono text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
        value={context.draft}
        onChange={(event) => context.setDraft(event.currentTarget.value)}
        disabled={context.busy}
        data-testid="agent-project-context-editor"
      />
    </div>
  );
}

function AgentProjectContextEntries({
  context,
  touchButton,
}: {
  context: ProjectContext;
  touchButton: string;
}) {
  const { t } = useTranslation();
  if (context.status === "loading") {
    return (
      <p role="status" className="px-2 py-3 text-sm text-muted-foreground">
        {t("common:loading")}
      </p>
    );
  }
  if (context.entries.length === 0) {
    return <p className="px-2 py-3 text-sm text-muted-foreground">{t("projects:emptyContext")}</p>;
  }
  return context.entries.map((entry) => (
    <button
      key={entry.path}
      type="button"
      className={cn(
        "flex w-full items-center gap-2 rounded px-2 text-left text-sm hover:bg-muted/60",
        touchButton,
      )}
      onClick={() => {
        if (entry.kind === "directory") void context.loadDirectory(entry.path);
        else void context.openFile(entry.path);
      }}
      disabled={context.busy}
    >
      {entry.kind === "directory" ? (
        <IconFolder className="h-4 w-4 shrink-0 text-muted-foreground" />
      ) : (
        <IconFile className="h-4 w-4 shrink-0 text-muted-foreground" />
      )}
      <span className="truncate">{entry.name}</span>
    </button>
  ));
}

function AgentProjectContextDirectory({
  context,
  touchButton,
}: {
  context: ProjectContext;
  touchButton: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex h-full min-h-0 flex-col" data-testid="agent-project-context">
      <div className="flex shrink-0 items-center gap-2 border-b px-3 py-2">
        {context.directory && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className={cn("shrink-0", touchButton)}
            onClick={context.goUp}
            aria-label={t("projects:goUpContext")}
          >
            <IconArrowLeft className="h-4 w-4" />
          </Button>
        )}
        <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
          {context.directory || t("projects:context")}
        </span>
      </div>
      {context.error && (
        <p role="alert" className="px-3 py-2 text-xs text-destructive">
          {context.error}
        </p>
      )}
      <div className="min-h-0 flex-1 overflow-y-auto p-1">
        <AgentProjectContextEntries context={context} touchButton={touchButton} />
      </div>
    </div>
  );
}

export function AgentProjectContextPanel({
  workspaceId,
  projectId,
}: {
  workspaceId: string;
  projectId: string;
}) {
  const { isMobile } = useResponsiveBreakpoint();
  const context = useAgentProjectContext(workspaceId, projectId);
  const touchButton = isMobile ? "min-h-11" : "min-h-7";
  return context.file ? (
    <AgentProjectContextFile context={context} touchButton={touchButton} />
  ) : (
    <AgentProjectContextDirectory context={context} touchButton={touchButton} />
  );
}
