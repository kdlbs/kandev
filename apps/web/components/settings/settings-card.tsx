import type { ComponentProps } from "react";
import { Card } from "@kandev/ui/card";

type SettingsCardProps = ComponentProps<typeof Card> & {
  isDirty?: boolean;
};

export function SettingsCard({ isDirty = false, ...props }: SettingsCardProps) {
  return <Card {...props} data-settings-dirty={isDirty} data-settings-dirty-level="card" />;
}
