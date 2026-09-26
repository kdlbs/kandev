import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";

type GitOperationResult = { success: boolean; output: string; error?: string };

/**
 * Resolve an operation name for `useGitWithFeedback`, optionally scoped to one
 * repository in a multi-repo task. The parenthesised repo form is a catalog
 * entry rather than a template literal so a locale can move or re-punctuate it.
 */
export function gitOperationLabel(
  // Deliberately not `TFunction`: callers include plain helpers that use the
  // module-level `t` from `@/lib/i18n`, whose signature is narrower.
  translate: (key: string, values?: Record<string, unknown>) => string,
  operationKey: string,
  repo?: string,
): string {
  const operation = translate(operationKey);
  return repo ? translate("common:gitOperationScoped", { operation, repo }) : operation;
}

/**
 * Wraps a git operation with toast feedback (loading → success/error).
 *
 * `operationName` is the ALREADY-TRANSLATED name of the operation ("Push",
 * "Rebase (kandev)"), which this hook interpolates into its own catalog
 * messages. Callers resolve it through `gitOperationLabel` below rather than
 * passing a literal: the three call sites used to pass English on purpose,
 * because this hook concatenated English around it and translating only one
 * half would have shipped a mixed-language toast.
 */
export function useGitWithFeedback() {
  const { t } = useTranslation();
  const { toast, updateToast } = useToast();

  const run = useCallback(
    async (operation: () => Promise<GitOperationResult>, operationName: string) => {
      const toastId = toast({
        title: t("common:gitOperationRunning", { operation: operationName }),
        variant: "loading",
      });
      try {
        const result = await operation();
        if (result.success) {
          updateToast(toastId, {
            title: t("common:gitOperationSucceeded", { operation: operationName }),
            description:
              result.output.slice(0, 200) ||
              t("common:gitOperationCompleted", { operation: operationName }),
            variant: "success",
          });
        } else {
          updateToast(toastId, {
            title: t("common:gitOperationFailed", { operation: operationName }),
            description: result.error || t("common:anErrorOccurred"),
            variant: "error",
          });
        }
      } catch (e) {
        updateToast(toastId, {
          title: t("common:gitOperationFailed", { operation: operationName }),
          description: e instanceof Error ? e.message : t("common:anUnexpectedErrorOccurred"),
          variant: "error",
        });
      }
    },
    [t, toast, updateToast],
  );

  return run;
}
