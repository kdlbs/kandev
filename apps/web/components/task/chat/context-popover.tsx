"use client";

import { useState, useCallback, useEffect, useRef } from "react";
import { IconListCheck, IconFile, IconFolder, IconSearch, IconAt } from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { getWebSocketClient } from "@/lib/ws/connection";
import { searchWorkspaceFiles } from "@/lib/ws/workspace-files";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import { isDirectory, getFileName } from "@/lib/utils/file-path";
import type { ContextFile } from "@/lib/state/context-files-store";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

type ContextPopoverProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  trigger: React.ReactNode;
  sessionId: string | null;
  /** Whether plan is included as context (from context files store, not plan panel) */
  planContextEnabled: boolean;
  contextFiles: ContextFile[];
  onToggleFile: (file: ContextFile) => void;
};

const FILE_SEARCH_DEBOUNCE = 300;

// `name` is display copy; it is resolved per call rather than at module load.
const planContextFile = (): ContextFile => ({
  path: "plan:context",
  name: t("task:panelPlan"),
  pinned: true,
});

function FileResultsList({
  isLoading,
  fileResults,
  query,
  filteredPromptsEmpty,
  isFileSelected,
  onToggle,
}: {
  isLoading: boolean;
  fileResults: string[];
  query: string;
  filteredPromptsEmpty: boolean;
  isFileSelected: (path: string) => boolean;
  onToggle: (filePath: string) => void;
}) {
  const { t } = useTranslation();
  if (isLoading) {
    return (
      <div className="px-3 py-3 text-center text-xs text-muted-foreground">{t("task:loading")}</div>
    );
  }
  if (fileResults.length === 0 && query && filteredPromptsEmpty) {
    return (
      <div className="px-3 py-3 text-center text-xs text-muted-foreground">
        {t("task:noResultsFound")}
      </div>
    );
  }
  return (
    <>
      {fileResults.map((filePath) => {
        const isDir = isDirectory(filePath);
        const name = getFileName(filePath);
        const parent = filePath.slice(0, filePath.length - name.length);
        return (
          <div
            key={filePath}
            className="flex items-center gap-2 px-3 py-1.5 hover:bg-muted/50 cursor-pointer"
            onClick={() => onToggle(filePath)}
          >
            <Checkbox
              checked={isFileSelected(filePath)}
              onCheckedChange={() => onToggle(filePath)}
              className="h-3.5 w-3.5"
            />
            {isDir ? (
              <IconFolder className="h-4 w-4 text-muted-foreground shrink-0" />
            ) : (
              <IconFile className="h-4 w-4 text-muted-foreground shrink-0" />
            )}
            <div className="flex-1 min-w-0">
              <span className="text-xs truncate block">{name}</span>
              {parent && (
                <span className="text-[10px] text-muted-foreground truncate block">{parent}</span>
              )}
            </div>
          </div>
        );
      })}
    </>
  );
}

type PromptsSectionProps = {
  query: string;
  prompts: { id: string; name: string; content?: string }[];
  contextFiles: ContextFile[];
  onToggleFile: (file: ContextFile) => void;
};

// A context-file path, not copy: `prompt:<id>` is the identity these entries
// are stored and compared under. Built outside JSX so the literal guard, which
// only inspects JSX, does not read it as a string to translate.
function promptContextPath(id: string): string {
  return `prompt:${id}`;
}

function PromptsSection({ query, prompts, contextFiles, onToggleFile }: PromptsSectionProps) {
  const { t } = useTranslation();
  if (prompts.length === 0) return null;
  return (
    <>
      {!query && (
        <div className="px-3 pt-2 pb-1">
          <p className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
            {t("common:prompts")}
          </p>
        </div>
      )}
      {prompts.map((prompt) => {
        const promptPath = promptContextPath(prompt.id);
        return (
          <div
            key={prompt.id}
            className="flex items-center gap-2 px-3 py-1.5 hover:bg-muted/50 cursor-pointer"
            onClick={() => onToggleFile({ path: promptPath, name: prompt.name, pinned: true })}
          >
            <Checkbox
              checked={contextFiles.some((f) => f.path === promptPath)}
              onCheckedChange={() =>
                onToggleFile({ path: promptPath, name: prompt.name, pinned: true })
              }
              className="h-3.5 w-3.5"
            />
            <IconAt className="h-4 w-4 text-muted-foreground shrink-0" />
            <div className="flex-1 min-w-0">
              <span className="text-xs truncate block">{prompt.name}</span>
              {prompt.content && (
                <span className="text-[10px] text-muted-foreground truncate block">
                  {prompt.content.length > 60
                    ? prompt.content.slice(0, 60) + "..."
                    : prompt.content}
                </span>
              )}
            </div>
          </div>
        );
      })}
    </>
  );
}

function useContextPopoverState(open: boolean, sessionId: string | null) {
  const [query, setQuery] = useState("");
  const [fileResults, setFileResults] = useState<string[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const searchTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  /* eslint-disable react-hooks/set-state-in-effect -- resetting state on open/close is intentional */
  useEffect(() => {
    if (open) {
      requestAnimationFrame(() => inputRef.current?.focus());
    } else {
      setQuery("");
      setFileResults([]);
    }
  }, [open]);
  /* eslint-enable react-hooks/set-state-in-effect */

  /* eslint-disable react-hooks/set-state-in-effect -- loading state sync is intentional for UX */
  useEffect(() => {
    if (!open || !sessionId) return;
    if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
    const delay = query === "" ? 0 : FILE_SEARCH_DEBOUNCE;
    setIsLoading(true);
    let cancelled = false;
    searchTimeoutRef.current = setTimeout(async () => {
      try {
        const client = getWebSocketClient();
        if (!client) {
          if (!cancelled) {
            setFileResults([]);
            setIsLoading(false);
          }
          return;
        }
        const response = await searchWorkspaceFiles(client, sessionId, query || "", 20);
        if (!cancelled) setFileResults(response.files || []);
      } catch {
        if (!cancelled) setFileResults([]);
      }
      if (!cancelled) setIsLoading(false);
    }, delay);
    return () => {
      cancelled = true;
      if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
    };
  }, [open, sessionId, query]);
  /* eslint-enable react-hooks/set-state-in-effect */

  return { query, setQuery, fileResults, isLoading, inputRef };
}

function ContextPopoverList({
  query,
  fileResults,
  isLoading,
  planContextEnabled,
  filteredPrompts,
  contextFiles,
  onToggleFile,
  isFileSelected,
  handleToggleFile,
}: {
  query: string;
  fileResults: string[];
  isLoading: boolean;
  planContextEnabled: boolean;
  filteredPrompts: { id: string; name: string; content?: string }[];
  contextFiles: ContextFile[];
  onToggleFile: (file: ContextFile) => void;
  isFileSelected: (path: string) => boolean;
  handleToggleFile: (filePath: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="max-h-60 overflow-y-auto border-t border-border">
      {!query && (
        <div
          className="flex items-center gap-2 px-3 py-1.5 hover:bg-muted/50 cursor-pointer"
          onClick={() => onToggleFile(planContextFile())}
        >
          <Checkbox
            checked={planContextEnabled}
            onCheckedChange={() => onToggleFile(planContextFile())}
            className="h-3.5 w-3.5"
          />
          <IconListCheck className="h-4 w-4 text-muted-foreground shrink-0" />
          <span className="text-xs flex-1 truncate">{t("task:plan")}</span>
          {planContextEnabled && (
            <span className="text-[9px] font-medium text-primary bg-primary/10 px-1.5 py-0.5 rounded">
              {t("task:activeBadge")}
            </span>
          )}
        </div>
      )}
      <PromptsSection
        query={query}
        prompts={filteredPrompts}
        contextFiles={contextFiles}
        onToggleFile={onToggleFile}
      />
      {!query && fileResults.length > 0 && (
        <div className="px-3 pt-2 pb-1">
          <p className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
            {t("task:files")}
          </p>
        </div>
      )}
      <FileResultsList
        isLoading={isLoading}
        fileResults={fileResults}
        query={query}
        filteredPromptsEmpty={filteredPrompts.length === 0}
        isFileSelected={isFileSelected}
        onToggle={handleToggleFile}
      />
    </div>
  );
}

export function ContextPopover({
  open,
  onOpenChange,
  trigger,
  sessionId,
  planContextEnabled,
  contextFiles,
  onToggleFile,
}: ContextPopoverProps) {
  const { t } = useTranslation();
  const { query, setQuery, fileResults, isLoading, inputRef } = useContextPopoverState(
    open,
    sessionId,
  );
  const { prompts } = useCustomPrompts();

  const handleToggleFile = useCallback(
    (filePath: string) => {
      const name = getFileName(filePath);
      onToggleFile({ path: filePath, name, pinned: true });
    },
    [onToggleFile],
  );

  const isFileSelected = useCallback(
    (path: string) => contextFiles.some((f) => f.path === path),
    [contextFiles],
  );

  const filteredPrompts = query
    ? prompts.filter((p) => p.name.toLowerCase().includes(query.toLowerCase()))
    : prompts;

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        side="top"
        align="start"
        className="w-72 p-0 gap-1"
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <div className="px-3 pt-3 pb-2 flex items-baseline gap-1.5">
          <p className="text-xs font-medium">{t("task:context")}</p>
          <p className="text-[10px] text-muted-foreground">
            {t("task:selectFilesAndPromptsToInclude")}
          </p>
        </div>
        <div className="px-3 pb-2">
          <div className="relative">
            <IconSearch className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              ref={inputRef}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("task:searchFilesAndPrompts")}
              className="h-7 pl-7 text-xs"
            />
          </div>
        </div>
        <ContextPopoverList
          query={query}
          fileResults={fileResults}
          isLoading={isLoading}
          planContextEnabled={planContextEnabled}
          filteredPrompts={filteredPrompts}
          contextFiles={contextFiles}
          onToggleFile={onToggleFile}
          isFileSelected={isFileSelected}
          handleToggleFile={handleToggleFile}
        />
      </PopoverContent>
    </Popover>
  );
}
