import { useTranslation } from "react-i18next";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";

type CoordinatorSwitcherProps = {
  coordinators: readonly Coordinator[];
  value: string;
  onChange: (coordinatorId: string) => void;
};

/** The header's coordinator select; shown only with two or more coordinators. */
export function CoordinatorSwitcher({ coordinators, value, onChange }: CoordinatorSwitcherProps) {
  const { t } = useTranslation();
  if (coordinators.length < 2) return null;
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger
        aria-label={t("coordinator:copilotSwitcherLabel")}
        data-testid="workspace-copilot-switcher"
        className="h-8 max-w-40 cursor-pointer [@media(pointer:coarse)]:h-11"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {coordinators.map((c) => (
          <SelectItem key={c.id} value={c.id} className="cursor-pointer">
            {c.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
