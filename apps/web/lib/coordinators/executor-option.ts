export type ExecutorOptionLike = {
  value: string;
  label: string;
  disabled?: boolean;
  disabledReason?: string;
};

// ExecutorProfileSelector has no analog to AgentProfilePicker's
// unavailable-value handling, so a stored executor profile id that no
// current profile resolves to (design B11, "same shape for Executor") needs
// a synthetic disabled option built here instead. Combobox already keeps a
// disabled current option first and shown as selected once it is in the list
// (see combobox.test.tsx), so prepending it is enough.
export function withUnavailableExecutorOption<T extends ExecutorOptionLike>(
  options: readonly T[],
  value: string,
  unavailableLabel: string,
): readonly T[] {
  if (!value || options.some((option) => option.value === value)) return options;
  const synthetic = {
    value,
    label: unavailableLabel,
    disabled: true,
    disabledReason: unavailableLabel,
  } as T;
  return [synthetic, ...options];
}
