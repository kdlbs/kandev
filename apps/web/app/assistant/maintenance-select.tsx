import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
export function MaintenanceSelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: Array<{ id: string; name: string }>;
  onChange: (value: string) => void;
}) {
  const id = useId();
  const { t } = useTranslation();
  return (
    <div className="space-y-1 min-w-0">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger id={id} className="w-full min-w-0 cursor-pointer max-md:min-h-11">
          <SelectValue placeholder={t("orchestration:maintenanceChoose")} />
        </SelectTrigger>
        <SelectContent>
          {options.map((row) => (
            <SelectItem key={row.id} value={row.id}>
              {row.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
