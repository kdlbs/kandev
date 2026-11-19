import { useEffect } from "react";

import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import {
  bootstrapImproveKandev,
  getImproveKandevBootstrapJob,
} from "@/lib/api/domains/improve-kandev-api";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import { t as tStatic } from "@/lib/i18n";
import type { BootstrapState } from "./improve-kandev-dialog-create";

const BOOTSTRAP_STATUS_TIMEOUT_MS = 30_000;

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
        let data = await bootstrapImproveKandev(workspaceId, {
          async: true,
          createWorkspace,
          init: { signal: controller.signal },
        });
        if (cancelled || controller.signal.aborted) return;
        while (isBootstrapJob(data)) {
          await waitForBootstrapPoll(controller.signal);
          data = await readBootstrapStatusWithTimeout(data.job_id, controller.signal);
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

function readBootstrapStatusWithTimeout(
  jobID: string,
  parentSignal: AbortSignal,
): ReturnType<typeof getImproveKandevBootstrapJob> {
  return new Promise((resolve, reject) => {
    const requestController = new AbortController();
    let settled = false;
    const cleanup = () => {
      window.clearTimeout(timeoutID);
      parentSignal.removeEventListener("abort", abortForParent);
    };
    const abortForParent = () => {
      if (settled) return;
      settled = true;
      requestController.abort();
      cleanup();
      reject(new Error());
    };
    const timeout = () => {
      if (settled) return;
      settled = true;
      requestController.abort();
      cleanup();
      reject(new Error(tStatic("common:bootstrapFailed")));
    };
    const timeoutID = window.setTimeout(timeout, BOOTSTRAP_STATUS_TIMEOUT_MS);
    if (parentSignal.aborted) {
      abortForParent();
      return;
    }
    parentSignal.addEventListener("abort", abortForParent, { once: true });
    getImproveKandevBootstrapJob(jobID, { init: { signal: requestController.signal } }).then(
      (response) => {
        if (settled) return;
        settled = true;
        cleanup();
        resolve(response);
      },
      (error: unknown) => {
        if (settled) return;
        settled = true;
        cleanup();
        reject(error);
      },
    );
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
      reject(new Error("bootstrap polling canceled"));
    };
    if (signal.aborted) {
      onAbort();
      return;
    }
    signal.addEventListener("abort", onAbort, { once: true });
  });
}
