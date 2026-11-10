import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { useAppStore } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import { IssueDetailSkeleton } from "./task-detail-skeleton";
import { resolveTaskIdentifier } from "@/lib/api/domains/office-extended-api";

const TASK_IDENTIFIER_PARAM_RE = /^.+-\d+$/;
const UUID_PARAM_RE = /^[\da-f]{8}(?:-[\da-f]{4}){3}-[\da-f]{12}$/i;

export type TaskIdentifierRedirectStatus =
  | "resolving"
  | "not-found"
  | "error"
  | "workspace-unavailable";

/** True when the route param ends in a task sequence and is not a UUID. */
export function isTaskIdentifierParam(param: string): boolean {
  return TASK_IDENTIFIER_PARAM_RE.test(param) && !UUID_PARAM_RE.test(param);
}

/**
 * Resolves an identifier-shaped route param to the task id and replaces the URL
 * with the canonical /office/tasks/<id>. Reports missing and transient failures
 * separately so only transient failures offer a retry action.
 */
export function useTaskIdentifierRedirect(identifier: string): {
  status: TaskIdentifierRedirectStatus;
  retry: () => void;
} {
  const router = useRouter();
  const searchParams = useSearchParams();
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  // A comment link names its own workspace; the active one is the fallback for
  // hand-typed URLs. Identifiers are unique per workspace only.
  const explicitWorkspaceId = searchParams.get("workspaceId") || searchParams.get("workspace");
  const workspaceId = explicitWorkspaceId || activeWorkspaceId;
  const searchQuery = searchParams.toString();
  const [status, setStatus] = useState<TaskIdentifierRedirectStatus>("resolving");
  const [retryCount, setRetryCount] = useState(0);

  useEffect(() => {
    if (!workspaceId) {
      setStatus("workspace-unavailable");
      return;
    }
    let cancelled = false;
    setStatus("resolving");
    resolveTaskIdentifier(workspaceId, identifier)
      .then(({ id }) => {
        if (cancelled) return;
        const redirectParams = new URLSearchParams(searchQuery);
        if (explicitWorkspaceId) {
          redirectParams.set("workspaceId", explicitWorkspaceId);
          redirectParams.delete("workspace");
        }
        const query = redirectParams.toString();
        router.replace(`/office/tasks/${id}${query ? `?${query}` : ""}`);
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        setStatus(error instanceof ApiError && error.status === 404 ? "not-found" : "error");
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, explicitWorkspaceId, searchQuery, identifier, router, retryCount]);

  const retry = () => setRetryCount((count) => count + 1);
  return { status: workspaceId ? status : "workspace-unavailable", retry };
}

/** Shown at an identifier-shaped URL while it resolves to the canonical task URL. */
export function TaskIdentifierRedirect({ identifier }: { identifier: string }) {
  const { t } = useTranslation();
  const router = useRouter();
  const { status, retry } = useTaskIdentifierRedirect(identifier);
  if (status === "resolving") return <IssueDetailSkeleton />;
  let message: string;
  switch (status) {
    case "not-found":
      message = t("office:taskNotFound");
      break;
    case "workspace-unavailable":
      message = t("office:taskIdentifierWorkspaceUnavailable");
      break;
    case "error":
      message = t("office:taskIdentifierLoadFailed");
      break;
  }
  return (
    <div className="flex h-full items-center justify-center">
      <div className="text-center">
        <p role="status" className="text-sm text-muted-foreground">
          {message}
        </p>
        <div className="mt-2 flex flex-col items-center gap-1">
          {status === "error" && (
            <button
              className="min-h-7 px-2 text-sm text-primary underline cursor-pointer [@media(pointer:coarse)]:min-h-11"
              onClick={retry}
            >
              {t("office:retry")}
            </button>
          )}
          <button
            className="min-h-7 px-2 text-sm text-primary underline cursor-pointer [@media(pointer:coarse)]:min-h-11"
            onClick={() => router.push("/office/tasks")}
          >
            {t("office:backToTasks")}
          </button>
        </div>
      </div>
    </div>
  );
}
