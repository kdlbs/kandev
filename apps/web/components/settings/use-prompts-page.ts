import { useCallback, useMemo, useState } from "react";

import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import { createPrompt, deletePrompt, updatePrompt } from "@/lib/api";
import { savePromptVersions } from "@/lib/api/domains/prompt-sync-api";
import { useRequest } from "@/lib/http/use-request";
import { t } from "@/lib/i18n";
import {
  isDraftValid,
  isSwitched,
  orphanedDrafts,
  planPromptSaves,
  showsEditor,
  withChoice,
  withDraftEdit,
  type PendingVersions,
  type PromptDrafts,
  type PromptFormState,
  type PromptSave,
} from "@/lib/settings/prompt-versions";
import type { CustomPrompt } from "@/lib/types/http";
import type { PromptConflictChoice } from "@/lib/types/prompt-sync";

export const emptyPromptForm: PromptFormState = { name: "", content: "" };

/** Open rows by prompt ID. */
type OpenRows = Record<string, true>;

function without<T>(record: Record<string, T>, ids: Iterable<string>): Record<string, T> {
  const next = { ...record };
  for (const id of ids) delete next[id];
  return next;
}

function errorMessage(error: unknown): string {
  // A thrown Error carries the backend's diagnostic, which stays English by
  // design; only the missing-payload fallback is copy.
  return error instanceof Error ? error.message : t("common:requestFailed");
}

export function usePromptsState() {
  const { loaded: promptsLoaded } = useCustomPrompts();
  const prompts = useAppStore((state) => state.prompts.items);
  const setPrompts = useAppStore((state) => state.setPrompts);
  const [showCreate, setShowCreate] = useState(false);
  const [createForm, setCreateForm] = useState(emptyPromptForm);
  const [openRows, setOpenRows] = useState<OpenRows>({});
  const [drafts, setDrafts] = useState<PromptDrafts>({});
  const [pending, setPending] = useState<PendingVersions>({});
  const [deleteTarget, setDeleteTarget] = useState<CustomPrompt | null>(null);
  return {
    promptsLoaded,
    prompts,
    setPrompts,
    showCreate,
    setShowCreate,
    createForm,
    setCreateForm,
    openRows,
    setOpenRows,
    drafts,
    setDrafts,
    pending,
    setPending,
    deleteTarget,
    setDeleteTarget,
  };
}

export type PromptsState = ReturnType<typeof usePromptsState>;

/** Drops prompts from every per-row state once they are saved, discarded or deleted. */
function useForgetRows({ setOpenRows, setDrafts, setPending }: PromptsState) {
  return useCallback(
    (ids: string[]) => {
      setOpenRows((current) => without(current, ids));
      setDrafts((current) => without(current, ids));
      setPending((current) => without(current, ids));
    },
    [setOpenRows, setDrafts, setPending],
  );
}

/** The prompt mutations, sharing the post-write list refresh. */
function usePromptRequests(state: PromptsState, closeCreate: () => void) {
  const store = useAppStoreApi();
  const { setPrompts } = state;
  const forgetRows = useForgetRows(state);
  const applyPrompts = useCallback(
    (change: (current: CustomPrompt[]) => CustomPrompt[]) => {
      setPrompts(
        change(store.getState().prompts.items).sort((a, b) => a.name.localeCompare(b.name)),
      );
    },
    [setPrompts, store],
  );

  const createAndList = async (form: PromptFormState) => {
    const prompt = await createPrompt(
      { name: form.name.trim(), content: form.content.trim() },
      { cache: "no-store" },
    );
    applyPrompts((current) => [...current.filter((p) => p.id !== prompt.id), prompt]);
  };

  const createRequest = useRequest(async (form: PromptFormState) => {
    await createAndList(form);
    closeCreate();
  });

  // saveRequest makes one Save's writes in order. Each written prompt closes
  // and leaves the drafts and switches at once, so a failure keeps only what
  // is unsaved.
  const saveRequest = useRequest(async (plan: PromptSave[]) => {
    for (const save of plan) {
      if (save.kind === "recreate") {
        await createAndList(save.form);
        forgetRows([save.id]);
        continue;
      }
      const updated =
        save.kind === "versions"
          ? await savePromptVersions(save.prompt.id, save.payload, { cache: "no-store" })
          : await updatePrompt(
              save.prompt.id,
              {
                name: save.form.name.trim(),
                content: save.form.content.trim(),
                expected_updated_at: save.expectedUpdatedAt,
              },
              { cache: "no-store" },
            );
      applyPrompts((current) => current.map((p) => (p.id === updated.id ? updated : p)));
      forgetRows([updated.id]);
    }
  });

  const deleteRequest = useRequest(async (id: string) => {
    await deletePrompt(id, { cache: "no-store" });
    applyPrompts((current) => current.filter((p) => p.id !== id));
    forgetRows([id]);
  });

  return { createRequest, saveRequest, deleteRequest, forgetRows };
}

async function runPromptSave(
  action: () => Promise<unknown>,
  reportError: (error: unknown) => void,
) {
  try {
    await action();
  } catch (error) {
    reportError(error);
    throw error;
  }
}

function useRowActions(state: PromptsState, forgetRows: (ids: string[]) => void) {
  const { setOpenRows, setDrafts, setPending } = state;
  return {
    toggleOpen: (prompt: CustomPrompt) =>
      setOpenRows((current) =>
        current[prompt.id] ? without(current, [prompt.id]) : { ...current, [prompt.id]: true },
      ),
    editForm: (prompt: CustomPrompt, patch: Partial<PromptFormState>) =>
      setDrafts((current) => withDraftEdit(current, prompt, patch)),
    // cancelEdit discards a row's text edit and closes it; its switch stays.
    cancelEdit: (prompt: CustomPrompt) => {
      setDrafts((current) => without(current, [prompt.id]));
      setOpenRows((current) => without(current, [prompt.id]));
    },
    choose: (prompt: CustomPrompt, choice: PromptConflictChoice) =>
      setPending((current) => withChoice(current, prompt, choice)),
    // discard resets the create form, every edit and every switch, closing the rows they touched.
    discardRows: () => forgetRows([...Object.keys(state.drafts), ...Object.keys(state.pending)]),
  };
}

export function usePromptsActions(state: PromptsState) {
  const { prompts, drafts, pending, showCreate, createForm, deleteTarget } = state;
  const { setShowCreate, setCreateForm, setOpenRows, setDeleteTarget } = state;
  const { toast } = useToast();

  const closeCreate = useCallback(() => {
    setShowCreate(false);
    setCreateForm(emptyPromptForm);
  }, [setShowCreate, setCreateForm]);

  const isValid = useMemo(() => {
    if (showCreate) return Boolean(createForm.name.trim() && createForm.content.trim());
    const listedValid = prompts.every(
      (prompt) => !drafts[prompt.id] || isDraftValid(prompt, drafts[prompt.id]!.form),
    );
    const orphansValid = orphanedDrafts(prompts, drafts).every(
      ({ form }) => form.name.trim() && form.content.trim(),
    );
    return listedValid && orphansValid;
  }, [showCreate, createForm, prompts, drafts]);

  const { createRequest, saveRequest, deleteRequest, forgetRows } = usePromptRequests(
    state,
    closeCreate,
  );
  const rows = useRowActions(state, forgetRows);
  const isBusy = [createRequest, saveRequest, deleteRequest].some((request) => request.isLoading);
  const toastError = (title: string) => (err: unknown) =>
    toast({ title, description: errorMessage(err), variant: "error" });

  return {
    ...rows,
    isValid,
    isBusy,
    closeCreate,
    handleCreate: () => {
      if (!isValid || isBusy) return;
      return runPromptSave(
        () => createRequest.run(createForm),
        toastError(t("settings:promptCreateFailed")),
      );
    },
    // handleUpdate saves every pending switch and every edited prompt.
    handleUpdate: () => {
      if (!isValid || isBusy) return;
      const plan = planPromptSaves({ prompts, pending, drafts });
      return runPromptSave(() => saveRequest.run(plan), toastError(t("settings:promptSaveFailed")));
    },
    discard: () => {
      closeCreate();
      rows.discardRows();
    },
    startCreate: () => {
      setOpenRows({});
      setCreateForm(emptyPromptForm);
      setShowCreate(true);
    },
    openDeleteDialog: (prompt: CustomPrompt) => setDeleteTarget(prompt),
    closeDeleteDialog: () => setDeleteTarget(null),
    confirmDelete: () => {
      if (!deleteTarget || isBusy) return;
      return deleteRequest.run(deleteTarget.id).catch(toastError(t("settings:promptDeleteFailed")));
    },
  };
}

/** The route draft: the create form, or every edit and every listed prompt's switch. */
export function getPromptDraftMeta({
  prompts,
  showCreate,
  createForm,
  drafts,
  pending,
}: Pick<PromptsState, "prompts" | "showCreate" | "createForm" | "drafts" | "pending">) {
  if (showCreate) return { isDirty: true, revision: `new:${JSON.stringify(createForm)}` };
  // Every draft counts, including one whose prompt was removed while edited,
  // so leaving the page still warns before its text is lost.
  const edits = Object.entries(drafts).map(([id, draft]) => [id, draft.form]);
  const switches = prompts
    .filter((prompt) => isSwitched(prompt, pending))
    .map((prompt) => [prompt.id, pending[prompt.id]]);
  return {
    isDirty: edits.length > 0 || switches.length > 0,
    revision: JSON.stringify({ edits, switches }),
  };
}

/** Rows that stay listed while a search hides others: edited, open for editing, or being deleted. */
export function activeRowIds(
  state: Pick<PromptsState, "prompts" | "openRows" | "drafts" | "pending" | "deleteTarget">,
) {
  const ids = new Set(Object.keys(state.drafts));
  for (const prompt of state.prompts) {
    if (state.openRows[prompt.id] && showsEditor(prompt, state.pending)) ids.add(prompt.id);
  }
  if (state.deleteTarget) ids.add(state.deleteTarget.id);
  return ids;
}
