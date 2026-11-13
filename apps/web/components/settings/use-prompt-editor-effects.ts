import { useEffect } from "react";
import type { Dispatch, SetStateAction } from "react";

import type { CustomPrompt } from "@/lib/types/http";

export type PromptFormState = { name: string; content: string };
export type PromptEditBaseline = Pick<CustomPrompt, "name" | "content" | "updated_at">;

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

  useEffect(() => {
    if (!editingPrompt || !editBaseline) return;
    const formIsPristine =
      formState.name === editBaseline.name && formState.content === editBaseline.content;
    if (!formIsPristine || editingPrompt.updated_at === editBaseline.updated_at) return;
    const nextBaseline = {
      name: editingPrompt.name,
      content: editingPrompt.content,
      updated_at: editingPrompt.updated_at,
    };
    setFormState({ name: nextBaseline.name, content: nextBaseline.content });
    setEditBaseline(nextBaseline);
  }, [
    editingPrompt?.content,
    editingPrompt?.name,
    editingPrompt?.updated_at,
    editBaseline,
    formState.content,
    formState.name,
    setEditBaseline,
    setFormState,
  ]);
}
