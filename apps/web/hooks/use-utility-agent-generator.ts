"use client";

import { useState, useCallback } from "react";
import { executeUtilityPrompt, type ExecutePromptRequest } from "@/lib/api/domains/utility-api";
import { useToast } from "@/components/toast-provider";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import type { FileInfo } from "@/lib/state/slices";

const ENHANCE_PROMPT = "enhance-prompt" as const;

type GeneratorType =
  | "commit-message"
  | "commit-description"
  | "pr-title"
  | "pr-description"
  | typeof ENHANCE_PROMPT;

const UTILITY_AGENT_IDS: Record<GeneratorType, string> = {
  "commit-message": "builtin-commit-message",
  "commit-description": "builtin-commit-description",
  "pr-title": "builtin-pr-title",
  "pr-description": "builtin-pr-description",
  [ENHANCE_PROMPT]: "builtin-enhance-prompt",
};

type UseUtilityAgentGeneratorOptions = {
  sessionId: string | null;
  taskTitle?: string;
  taskDescription?: string;
};

type GenerateOptions = {
  onSuccess?: (content: string) => void;
  // Additional context for PR description
  commitLog?: string;
  diffSummary?: string;
  // User's original prompt for enhancement
  userPrompt?: string;
};

export function useUtilityAgentGenerator({
  sessionId,
  taskTitle,
  taskDescription,
}: UseUtilityAgentGeneratorOptions) {
  const [generating, setGenerating] = useState<Set<GeneratorType>>(new Set());
  const { toast } = useToast();
  const gitStatus = useSessionGitStatus(sessionId);

  const collectGitContext = useCallback(() => {
    const changedFiles = gitStatus?.files ? Object.keys(gitStatus.files) : [];
    const diffs = gitStatus?.files
      ? Object.values(gitStatus.files as Record<string, FileInfo>)
          .filter((f) => f.diff)
          .map((f) => f.diff)
          .join("\n\n")
      : undefined;
    return { changedFiles, diffs };
  }, [gitStatus]);

  const clearType = useCallback(
    (type: GeneratorType) =>
      setGenerating((prev) => {
        const next = new Set(prev);
        next.delete(type);
        return next;
      }),
    [],
  );

  const buildRequest = useCallback(
    (type: GeneratorType, options?: GenerateOptions): ExecutePromptRequest => {
      const sessionless = type === ENHANCE_PROMPT && !sessionId;
      const { changedFiles, diffs } = sessionless
        ? { changedFiles: [] as string[], diffs: undefined }
        : collectGitContext();
      return {
        utility_agent_id: UTILITY_AGENT_IDS[type],
        session_id: sessionId ?? "",
        task_title: taskTitle,
        task_description: taskDescription,
        git_diff: diffs || undefined,
        changed_files: changedFiles.join("\n") || undefined,
        commit_log: options?.commitLog,
        diff_summary: options?.diffSummary,
        user_prompt: options?.userPrompt,
      };
    },
    [sessionId, taskTitle, taskDescription, collectGitContext],
  );

  const generate = useCallback(
    async (type: GeneratorType, options?: GenerateOptions) => {
      // enhance-prompt can run sessionless via the host utility manager.
      // Other generators need git context from an active session.
      if (!sessionId && type !== ENHANCE_PROMPT) {
        toast({
          title: "No active session",
          description: "Start a session first to use AI generation",
          variant: "error",
        });
        return;
      }

      setGenerating((prev) => new Set(prev).add(type));
      try {
        const resp = await executeUtilityPrompt(buildRequest(type, options));
        clearType(type);
        if (resp.success && resp.response) {
          const response = resp.response;
          requestAnimationFrame(() => options?.onSuccess?.(response));
        } else {
          toast({
            title: "Generation failed",
            description: resp.error || "Failed to generate content",
            variant: "error",
          });
        }
      } catch (error) {
        clearType(type);
        toast({
          title: "Generation failed",
          description: error instanceof Error ? error.message : "Unknown error",
          variant: "error",
        });
      }
    },
    [sessionId, buildRequest, clearType, toast],
  );

  return useGeneratorCallbacks(generate, generating);
}

function useGeneratorCallbacks(
  generate: (type: GeneratorType, options?: GenerateOptions) => Promise<void>,
  generating: Set<GeneratorType>,
) {
  const generateCommitMessage = useCallback(
    (onSuccess: (message: string) => void) => generate("commit-message", { onSuccess }),
    [generate],
  );

  const generateCommitDescription = useCallback(
    (onSuccess: (description: string) => void) => generate("commit-description", { onSuccess }),
    [generate],
  );

  const generatePRTitle = useCallback(
    (onSuccess: (title: string) => void, extra?: { commitLog?: string; diffSummary?: string }) =>
      generate("pr-title", { onSuccess, ...extra }),
    [generate],
  );

  const generatePRDescription = useCallback(
    (
      onSuccess: (description: string) => void,
      extra?: { commitLog?: string; diffSummary?: string },
    ) => generate("pr-description", { onSuccess, ...extra }),
    [generate],
  );

  const enhancePrompt = useCallback(
    (userPrompt: string, onSuccess: (enhanced: string) => void) =>
      generate(ENHANCE_PROMPT, { onSuccess, userPrompt }),
    [generate],
  );

  return {
    isGenerating: generating.size > 0,
    isGeneratingCommitMessage: generating.has("commit-message"),
    isGeneratingCommitDescription: generating.has("commit-description"),
    isGeneratingPRTitle: generating.has("pr-title"),
    isGeneratingPRDescription: generating.has("pr-description"),
    isEnhancingPrompt: generating.has(ENHANCE_PROMPT),
    generateCommitMessage,
    generateCommitDescription,
    generatePRTitle,
    generatePRDescription,
    enhancePrompt,
  };
}
