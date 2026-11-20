import { useEffect } from "react";
import type { Dispatch, SetStateAction } from "react";

import { formFromPrompt, type PromptFormState } from "@/lib/settings/prompt-versions";
import type { CustomPrompt } from "@/lib/types/http";

export type PromptEditBaseline = PromptFormState & Pick<CustomPrompt, "updated_at">;

type PromptEditorEffectsOptions = {
  editingId: string | null;
  editingRef: { current: HTMLDivElement | null };
  editingPrompt: CustomPrompt | undefined;
  editBaseline: PromptEditBaseline | null;
  formState: PromptFormState;
  setEditBaseline: Dispatch<SetStateAction<PromptEditBaseline | null>>;
  setFormState: Dispatch<SetStateAction<PromptFormState>>;
};

export function usePromptEditorEffects({
  editingId,
  editingRef,
  editingPrompt,
  editBaseline,
  formState,
  setEditBaseline,
  setFormState,
}: PromptEditorEffectsOptions) {
  useEffect(() => {
    if (!editingId) return;
    editingRef.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, [editingId, editingRef]);

  // A pristine draft follows a newer saved revision of the prompt it edits.
  useEffect(() => {
    if (!editingPrompt || !editBaseline) return;
    if (editingPrompt.updated_at === editBaseline.updated_at) return;
    const { updated_at: _revision, ...baselineForm } = editBaseline;
    if (JSON.stringify(formState) !== JSON.stringify(baselineForm)) return;
    const next = formFromPrompt(editingPrompt);
    setFormState(next);
    setEditBaseline({ ...next, updated_at: editingPrompt.updated_at });
  }, [editingPrompt, editBaseline, formState, setEditBaseline, setFormState]);
}
