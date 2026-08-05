"use client";

import type { ComponentProps } from "react";
import { useSettingsTargetRegistration } from "./settings-target-provider";

type SettingsTargetProps = ComponentProps<"div"> & {
  targetId: string;
};

export function SettingsTarget({ targetId, ...props }: SettingsTargetProps) {
  const registerTarget = useSettingsTargetRegistration(targetId);
  return <div {...props} ref={registerTarget} />;
}
