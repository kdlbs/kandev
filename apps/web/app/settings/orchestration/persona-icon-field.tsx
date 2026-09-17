import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { AgentAvatar } from "@/components/shared/agent-avatar";
export function PersonaIconField({
  name,
  icon,
  onChange,
  disabled,
}: {
  disabled?: boolean;
  name: string;
  icon?: string;
  onChange: (icon: string) => void;
}) {
  const id = useId();
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{t("orchestration:personaIcon")}</Label>
      <div className="flex items-center gap-3">
        <AgentAvatar name={name} icon={icon} />
        <Select
          disabled={disabled}
          value={icon || "initials"}
          onValueChange={(value) => onChange(value === "initials" ? "" : value)}
        >
          <SelectTrigger id={id}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="initials">{t("orchestration:nameInitials")}</SelectItem>
            {["💼", "🧭", "🤖", "🛠️", "🌱", "⭐"].map((value) => (
              <SelectItem value={value} key={value}>
                {value}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}
