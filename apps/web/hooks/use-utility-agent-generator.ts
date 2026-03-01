"use client";

import { useState, useCallback } from "react";
import { executeUtilityPrompt, type ExecutePromptRequest } from "@/lib/api/domains/utility-api";
import { useToast } from "@/components/toast-provider";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import type { FileInfo } from "@/lib/state/slices";

type GeneratorType = "commit-message" | "pr-description" | "enhance-prompt";

const UTILITY_AGENT_IDS: Record<GeneratorType, string> = {
  "commit-message": "builtin-commit-message",
  "pr-description": "builtin-pr-description",
  "enhance-prompt": "builtin-enhance-prompt",
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
  const [isGenerating, setIsGenerating] = useState<GeneratorType | null>(null);
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

  const generate = useCallback(
    async (type: GeneratorType, options?: GenerateOptions) => {
      if (!sessionId) {
        toast({
          title: "No active session",
          description: "Start a session first to use AI generation",
          variant: "error",
        });
        return;
      }

      setIsGenerating(type);
      try {
        const { changedFiles, diffs } = collectGitContext();

        const request: ExecutePromptRequest = {
          utility_agent_id: UTILITY_AGENT_IDS[type],
          session_id: sessionId,
          task_title: taskTitle,
          task_description: taskDescription,
          git_diff: diffs || undefined,
          changed_files: changedFiles.join("\n") || undefined,
          commit_log: options?.commitLog,
          diff_summary: options?.diffSummary,
          user_prompt: options?.userPrompt,
        };

        const resp = await executeUtilityPrompt(request);

        // Clear generating state BEFORE calling onSuccess so the editor is re-enabled
        setIsGenerating(null);

        if (resp.success && resp.response) {
          // Use requestAnimationFrame to ensure React has re-rendered with editor enabled
          const response = resp.response;
          requestAnimationFrame(() => {
            options?.onSuccess?.(response);
          });
        } else {
          toast({
            title: "Generation failed",
            description: resp.error || "Failed to generate content",
            variant: "error",
          });
        }
      } catch (error) {
        setIsGenerating(null);
        toast({
          title: "Generation failed",
          description: error instanceof Error ? error.message : "Unknown error",
          variant: "error",
        });
      }
    },
    [sessionId, taskTitle, taskDescription, collectGitContext, toast],
  );

  const generateCommitMessage = useCallback(
    (onSuccess: (message: string) => void) => {
      return generate("commit-message", { onSuccess });
    },
    [generate],
  );

  const generatePRDescription = useCallback(
    (
      onSuccess: (description: string) => void,
      extra?: { commitLog?: string; diffSummary?: string },
    ) => {
      return generate("pr-description", { onSuccess, ...extra });
    },
    [generate],
  );

  const enhancePrompt = useCallback(
    (userPrompt: string, onSuccess: (enhanced: string) => void) => {
      return generate("enhance-prompt", { onSuccess, userPrompt });
    },
    [generate],
  );

  return {
    isGenerating,
    isGeneratingCommitMessage: isGenerating === "commit-message",
    isGeneratingPRDescription: isGenerating === "pr-description",
    isEnhancingPrompt: isGenerating === "enhance-prompt",
    generateCommitMessage,
    generatePRDescription,
    enhancePrompt,
  };
}
