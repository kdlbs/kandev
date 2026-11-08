import { getBuiltinMCPSession } from "@/lib/api/domains/builtin-mcp-api";
import type { useToast } from "@/components/toast-provider";
import type { useTranslation } from "react-i18next";

export async function checkWalkthroughPackage(
  taskId: string,
  sessionId: string,
  current: () => boolean,
  toast: ReturnType<typeof useToast>["toast"],
  t: ReturnType<typeof useTranslation>["t"],
): Promise<boolean> {
  const available = await getBuiltinMCPSession(sessionId);
  if (!current()) return false;
  if (
    available.task_id !== taskId ||
    available.session_id !== sessionId ||
    available.walkthrough_availability !== "available"
  ) {
    toast({
      title: t("settings:builtinMcp.walkthroughUnavailable"),
      description: t("settings:builtinMcp.walkthroughRecovery"),
      variant: "error",
      action: {
        label: t("settings:builtinMcp.refresh"),
        onClick: () => {
          void getBuiltinMCPSession(sessionId)
            .then((next) => {
              if (current())
                toast({
                  title: t(
                    next.walkthrough_availability === "available"
                      ? "settings:builtinMcp.walkthroughReady"
                      : "settings:builtinMcp.walkthroughUnavailable",
                  ),
                });
            })
            .catch(() => {
              if (current())
                toast({ title: t("review:failedToRequestWalkthrough"), variant: "error" });
            });
        },
      },
    });
    return false;
  }
  return true;
}
