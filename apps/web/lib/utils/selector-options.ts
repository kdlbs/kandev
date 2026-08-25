/** Return a new list with the current option first when it is present. */
export function prioritizeSelectedOption<T>(
  options: readonly T[],
  selectedValue: string | null | undefined,
  getValue: (option: T) => string,
): T[] {
  if (!selectedValue) return [...options];

  const selectedIndex = options.findIndex((option) => getValue(option) === selectedValue);
  if (selectedIndex <= 0) return [...options];

  const selected = options[selectedIndex];
  if (selected === undefined) return [...options];

  return [selected, ...options.slice(0, selectedIndex), ...options.slice(selectedIndex + 1)];
}

export function prioritizeValueOption<T extends { value: string }>(
  options: readonly T[],
  selectedValue: string | null | undefined,
): T[] {
  return prioritizeSelectedOption(options, selectedValue, (option) => option.value);
}

export function prioritizeIdOption<T extends { id: string }>(
  options: readonly T[],
  selectedValue: string | null | undefined,
): T[] {
  return prioritizeSelectedOption(options, selectedValue, (option) => option.id);
}

export function selectorOptionClassName(selected: boolean, disabled = false): string {
  return [
    "relative min-h-11 border border-transparent pr-7 sm:min-h-7",
    selected &&
      "border-primary/50 bg-card font-medium data-[selected=true]:ring-2 data-[selected=true]:ring-primary/40",
    disabled && "opacity-40 cursor-not-allowed",
  ]
    .filter(Boolean)
    .join(" ");
}

export function configClassName(selected: boolean): string {
  return [
    "h-auto min-h-11 w-full min-w-0 cursor-pointer justify-start border border-transparent px-2 py-2 text-left sm:min-h-9",
    selected && "border-primary/50 bg-card font-medium hover:bg-card",
  ]
    .filter(Boolean)
    .join(" ");
}
