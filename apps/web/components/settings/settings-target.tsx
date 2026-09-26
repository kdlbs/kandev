"use client";

import { useCallback, type ComponentProps, type Ref } from "react";
import { useSettingsTargetRegistration } from "./settings-target-provider";

type SettingsTargetProps = ComponentProps<"div"> & {
  targetId: string;
};

export function SettingsTarget({ targetId, ref, ...props }: SettingsTargetProps) {
  const registerTarget = useSettingsTargetRegistration(targetId);
  const mergedRef = useCallback(
    (element: HTMLDivElement | null) => {
      registerTarget(element);
      assignRef(ref, element);
    },
    [ref, registerTarget],
  );
  return <div {...props} ref={mergedRef} />;
}

function assignRef(ref: Ref<HTMLDivElement> | undefined, element: HTMLDivElement | null): void {
  if (typeof ref === "function") ref(element);
  else if (ref) ref.current = element;
}
