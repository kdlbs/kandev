import { useEffect } from "react";

import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import {
  bootstrapImproveKandev,
  getImproveKandevBootstrapJob,
  type ImproveKandevBootstrapResponse,
} from "@/lib/api/domains/improve-kandev-api";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import { t as tStatic } from "@/lib/i18n";
import type { WorkflowStep } from "@/lib/types/http";

const BOOTSTRAP_REQUEST_TIMEOUT_MS = 30_000;

export type BootstrapState =
  | { kind: "idle" }
  | { kind: "loading" }
  | {
      kind: "ready";
      data: ImproveKandevBootstrapResponse;
      steps: WorkflowStep[];
      issueSteps: WorkflowStep[];
    }
  | { kind: "error"; message: string };

type UseBootstrapKandevOptions = {
  open: boolean;
  mode: "intro" | "create";
  workspaceId: string | null;
  gate: { workspaceChoiceConfirmed: boolean; createWorkspace: boolean };
  setBootstrap: (state: BootstrapState) => void;
  retry: number;
};

export function useBootstrapKandev({
  open,
  mode,
  workspaceId,
  gate,
  setBootstrap,
  retry,
}: UseBootstrapKandevOptions) {
  const { toast } = useToast();
  const setRepositories = useAppStore((state) => state.setRepositories);
  const { workspaceChoiceConfirmed, createWorkspace } = gate;
  useEffect(() => {
    if (!open || mode !== "create" || !workspaceId || !workspaceChoiceConfirmed) return;
    let cancelled = false;
    const controller = new AbortController();
    setBootstrap({ kind: "loading" });
    (async () => {
      try {
        let data = await requestBootstrapWithTimeout(controller.signal, (signal) =>
          bootstrapImproveKandev(workspaceId, {
            async: true,
            createWorkspace,
            init: { signal },
          }),
        );
        if (cancelled || controller.signal.aborted) return;
        while (isBootstrapJob(data)) {
          const jobID = data.job_id;
          await waitForBootstrapPoll(controller.signal);
          data = await requestBootstrapWithTimeout(controller.signal, (signal) =>
            getImproveKandevBootstrapJob(jobID, { init: { signal } }),
          );
          if (cancelled || controller.signal.aborted) return;
        }
        // Refresh the dedicated workspace's repository list so the newly-created
        // kandev repo is in the store; otherwise the locked repo dropdown can't
        // resolve a label for the bootstrapped repository_id. All improve work
        // is scoped to the workspace the bootstrap returns, not the active one.
        const [stepsRes, issueStepsRes, reposRes] = await Promise.all([
          listWorkflowSteps(data.workflow_id),
          listWorkflowSteps(data.issue_workflow_id),
          listRepositories(data.workspace_id, undefined, { cache: "no-store" }),
        ]);
        if (cancelled) return;
        setRepositories(data.workspace_id, reposRes.repositories);
        setBootstrap({
          kind: "ready",
          data,
          steps: stepsRes.steps,
          issueSteps: issueStepsRes.steps,
        });
      } catch (err) {
        if (cancelled || controller.signal.aborted) return;
        const message = err instanceof Error ? err.message : tStatic("common:bootstrapFailed");
        setBootstrap({ kind: "error", message });
        toast({
          title: tStatic("common:couldNotPrepareImproveKandev"),
          description: message,
          variant: "error",
        });
      }
    })();
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [
    open,
    mode,
    workspaceId,
    workspaceChoiceConfirmed,
    createWorkspace,
    setBootstrap,
    setRepositories,
    toast,
    retry,
  ]);
}

function isBootstrapJob(
  result: Awaited<ReturnType<typeof bootstrapImproveKandev>>,
): result is { job_id: string } {
  return "job_id" in result;
}

function requestBootstrapWithTimeout<T>(
  parentSignal: AbortSignal,
  request: (signal: AbortSignal) => Promise<T>,
): Promise<T> {
  return new Promise((resolve, reject) => {
    const requestController = new AbortController();
    if (parentSignal.aborted) {
      requestController.abort();
      reject(new Error());
      return;
    }
    let settled = false;
    const timeoutID = window.setTimeout(onTimeout, BOOTSTRAP_REQUEST_TIMEOUT_MS);
    function cleanup() {
      window.clearTimeout(timeoutID);
      parentSignal.removeEventListener("abort", abortForParent);
    }
    function abortForParent() {
      if (settled) return;
      settled = true;
      requestController.abort();
      cleanup();
      reject(new Error());
    }
    function onTimeout() {
      if (settled) return;
      settled = true;
      requestController.abort();
      cleanup();
      reject(new Error(tStatic("common:bootstrapFailed")));
    }
    function fail(error: unknown) {
      if (settled) return;
      settled = true;
      cleanup();
      reject(error);
    }
    parentSignal.addEventListener("abort", abortForParent, { once: true });
    try {
      request(requestController.signal).then((response) => {
        if (settled) return;
        settled = true;
        cleanup();
        resolve(response);
      }, fail);
    } catch (error) {
      fail(error);
    }
  });
}

function waitForBootstrapPoll(signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = window.setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, 1000);
    const onAbort = () => {
      window.clearTimeout(timer);
      signal.removeEventListener("abort", onAbort);
      reject(new Error());
    };
    if (signal.aborted) {
      onAbort();
      return;
    }
    signal.addEventListener("abort", onAbort, { once: true });
  });
}
