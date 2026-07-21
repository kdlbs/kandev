import type { Dispatch, SetStateAction } from "react";

export function synchronizeInputValue(
  inputValueRef: { current: string },
  setInput: Dispatch<SetStateAction<string>>,
  next: SetStateAction<string>,
) {
  const nextValue = typeof next === "function" ? next(inputValueRef.current) : next;
  inputValueRef.current = nextValue;
  setInput(nextValue);
}
