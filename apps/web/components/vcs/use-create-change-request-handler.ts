import { useCallback, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { getChangeRequestTerminology, type PRCreateResult } from "@/hooks/use-git-operations";
import { openExternalLink } from "@/lib/desktop/external-links";
import {
  getChangeRequestFailureFeedback,
  getChangeRequestSuccessFeedback,
} from "./change-request-feedback";

export type CreateChangeRequestInput = {
  title: string;
  body: string;
  baseBranch?: string;
  draft: boolean;
  repositoryScope?: string;
  branchAlreadyPushed: boolean;
  signal: AbortSignal;
};

export type CreateChangeRequest = (input: CreateChangeRequestInput) => Promise<PRCreateResult>;

type ChangeRequestDialogState = {
  title: string;
  setTitle: (value: string) => void;
  body: string;
  setBody: (value: string) => void;
  draft: boolean;
  branchPushed: boolean;
  setBranchPushed: (value: boolean) => void;
  repo: string | undefined;
  setOpen: (value: boolean) => void;
};

function openCreatedChangeRequest({
  url,
  taskId,
  repositoryScope,
  rememberURL,
}: {
  url: string | undefined;
  taskId: string | null;
  repositoryScope: string | undefined;
  rememberURL: (taskId: string, repositoryScope: string, url: string) => void;
}) {
  if (!url) return;
  if (taskId) rememberURL(taskId, repositoryScope || "", url);
  void openExternalLink(url).catch(() => undefined);
}

export function useCreateChangeRequestHandler({
  dialog,
  baseBranch,
  createChangeRequest,
  defaultTerminology,
  supportsDraft,
}: {
  dialog: ChangeRequestDialogState;
  baseBranch: string | undefined;
  createChangeRequest: CreateChangeRequest;
  defaultTerminology: ReturnType<typeof getChangeRequestTerminology>;
  supportsDraft: boolean;
}) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const setPendingPrUrlForTask = useAppStore((state) => state.setPendingPrUrlForTask);
  const requestController = useRef<AbortController | null>(null);

  useEffect(
    () => () => {
      requestController.current?.abort();
      requestController.current = null;
    },
    [],
  );

  return useCallback(async () => {
    if (!dialog.title.trim()) return;
    requestController.current?.abort();
    const controller = new AbortController();
    requestController.current = controller;
    try {
      dialog.setOpen(false);
      try {
        const effectiveDraft = supportsDraft && dialog.draft;
        const result = await createChangeRequest({
          title: dialog.title.trim(),
          body: dialog.body.trim(),
          baseBranch,
          draft: effectiveDraft,
          repositoryScope: dialog.repo,
          branchAlreadyPushed: dialog.branchPushed,
          signal: controller.signal,
        });
        if (controller.signal.aborted) return;
        if (result.success) {
          toast(getChangeRequestSuccessFeedback(result, effectiveDraft, defaultTerminology));
          openCreatedChangeRequest({
            url: result.pr_url,
            taskId: activeTaskId,
            repositoryScope: dialog.repo,
            rememberURL: setPendingPrUrlForTask,
          });
        } else {
          toast(getChangeRequestFailureFeedback(result, defaultTerminology));
          if (result.branch_pushed) {
            dialog.setBranchPushed(true);
            dialog.setOpen(true);
            return;
          }
        }
      } catch (error) {
        if (controller.signal.aborted) return;
        toast({
          title: t("integrations:createFailed", { shortName: defaultTerminology.shortName }),
          description: error instanceof Error ? error.message : t("integrations:anErrorOccurred"),
          variant: "error",
        });
      }
      if (controller.signal.aborted) return;
      dialog.setTitle("");
      dialog.setBody("");
      dialog.setBranchPushed(false);
    } finally {
      if (requestController.current === controller) requestController.current = null;
    }
  }, [
    activeTaskId,
    baseBranch,
    createChangeRequest,
    defaultTerminology,
    dialog,
    setPendingPrUrlForTask,
    supportsDraft,
    t,
    toast,
  ]);
}
