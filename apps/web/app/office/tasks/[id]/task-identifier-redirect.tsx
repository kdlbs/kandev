import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { useAppStore } from "@/components/state-provider";
import { IssueDetailSkeleton } from "./task-detail-skeleton";
import { resolveTaskIdentifier } from "@/lib/api/domains/office-extended-api";

const TASK_IDENTIFIER_PARAM_RE = /^[A-Z][A-Z0-9]*-\d+$/;

/** True when the route param looks like a workspace task identifier (KAN-11), not a UUID. */
export function isTaskIdentifierParam(param: string): boolean {
  return TASK_IDENTIFIER_PARAM_RE.test(param);
}

/**
 * Resolves an identifier-shaped route param to the task id and replaces the URL
 * with the canonical /office/tasks/<id>. Returns "failed" when no task matches.
 */
function useTaskIdentifierRedirect(identifier: string): "resolving" | "failed" {
  const router = useRouter();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    setFailed(false);
    resolveTaskIdentifier(workspaceId, identifier)
      .then(({ id }) => {
        if (!cancelled) router.replace(`/office/tasks/${id}`);
      })
      .catch(() => {
        if (!cancelled) setFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, identifier, router]);

  return failed ? "failed" : "resolving";
}

/** Shown at an identifier-shaped URL while it resolves to the canonical task URL. */
export function TaskIdentifierRedirect({ identifier }: { identifier: string }) {
  const { t } = useTranslation();
  const router = useRouter();
  const status = useTaskIdentifierRedirect(identifier);
  if (status === "resolving") return <IssueDetailSkeleton />;
  return (
    <div className="flex h-full items-center justify-center">
      <div className="text-center">
        <p className="text-sm text-muted-foreground">{t("office:taskNotFound")}</p>
        <button
          className="mt-2 text-sm text-primary underline cursor-pointer"
          onClick={() => router.push("/office/tasks")}
        >
          {t("office:backToTasks")}
        </button>
      </div>
    </div>
  );
}
