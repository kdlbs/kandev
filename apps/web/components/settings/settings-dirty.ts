export function isSetMembershipDirty(
  draft: readonly string[],
  baseline: readonly string[],
): boolean {
  const draftValues = new Set(draft);
  const baselineValues = new Set(baseline);
  if (draftValues.size !== baselineValues.size) return true;
  return [...draftValues].some((value) => !baselineValues.has(value));
}

export function isDraftEntryDirty(
  draft: Readonly<Record<string, string>>,
  baseline: Readonly<Record<string, string>>,
  key: string,
): boolean {
  return (draft[key] ?? "") !== (baseline[key] ?? "");
}

type EditorsDirtyState = {
  defaultEditorId: string;
  baselineDefaultId: string;
  lspAutoStartLanguages: readonly string[];
  baselineLspAutoStart: readonly string[];
  lspAutoInstallLanguages: readonly string[];
  baselineLspAutoInstall: readonly string[];
  lspConfigStrings: Readonly<Record<string, string>>;
  baselineLspConfigStrings: Readonly<Record<string, string>>;
};

export function isEditorsSettingsDirty(state: EditorsDirtyState): boolean {
  return (
    state.defaultEditorId !== state.baselineDefaultId ||
    isSetMembershipDirty(state.lspAutoStartLanguages, state.baselineLspAutoStart) ||
    isSetMembershipDirty(state.lspAutoInstallLanguages, state.baselineLspAutoInstall) ||
    JSON.stringify(state.lspConfigStrings) !== JSON.stringify(state.baselineLspConfigStrings)
  );
}
