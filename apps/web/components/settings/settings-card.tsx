"use client";

import { useCallback, type ComponentProps, type Ref } from "react";
import { Card } from "@kandev/ui/card";
import { useSettingsTargetRegistration } from "./settings-target-provider";

type SettingsCardProps = ComponentProps<typeof Card> & {
  isDirty?: boolean;
  discoveryTargetId?: string;
};

export function SettingsCard({
  isDirty = false,
  discoveryTargetId,
  ref,
  ...props
}: SettingsCardProps) {
  const registerTarget = useSettingsTargetRegistration(discoveryTargetId);
  const mergedRef = useCallback(
    (element: HTMLDivElement | null) => {
      registerTarget(element);
      assignRef(ref, element);
    },
    [ref, registerTarget],
  );
  return (
    <Card
      {...props}
      ref={mergedRef}
      data-settings-dirty={isDirty}
      data-settings-dirty-level="card"
    />
  );
}

function assignRef(ref: Ref<HTMLDivElement> | undefined, element: HTMLDivElement | null): void {
  if (typeof ref === "function") ref(element);
  else if (ref) ref.current = element;
}
