import { useCallback } from "react";
import { useToast } from "@/components/toast-provider";

type GitOperationResult = { success: boolean; output: string; error?: string };

/**
 * Wraps a git operation with toast feedback (loading → success/error).
 */
export function useGitWithFeedback() {
  const { toast, updateToast } = useToast();

  const run = useCallback(
    async (operation: () => Promise<GitOperationResult>, operationName: string) => {
      const toastId = toast({
        title: `${operationName}...`,
        variant: "loading",
      });
      try {
        const result = await operation();
        if (result.success) {
          updateToast(toastId, {
            title: `${operationName} successful`,
            description: result.output.slice(0, 200) || `${operationName} completed`,
            variant: "success",
          });
        } else {
          updateToast(toastId, {
            title: `${operationName} failed`,
            description: result.error || "An error occurred",
            variant: "error",
          });
        }
      } catch (e) {
        updateToast(toastId, {
          title: `${operationName} failed`,
          description: e instanceof Error ? e.message : "An unexpected error occurred",
          variant: "error",
        });
      }
    },
    [toast, updateToast],
  );

  return run;
}
