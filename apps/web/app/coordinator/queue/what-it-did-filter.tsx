import { useTranslation } from "react-i18next";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { ACTIVITY_CLASSES, type ActivityClass } from "@/lib/api/domains/coordinator-activity-api";
import { CLASS_LABEL_KEY } from "./activity-text";

const ALL_VALUE = "all";

export type WhatItDidFilterProps = {
  value: ActivityClass | undefined;
  onChange: (value: ActivityClass | undefined) => void;
};

/** The action class filter: All, the six classes and "Unknown action". */
export function WhatItDidFilter({ value, onChange }: WhatItDidFilterProps) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2 text-xs">
      <span className="text-muted-foreground" id="activity-filter-label">
        {t("coordinator:activityFilterLabel")}
      </span>
      <Select
        value={value ?? ALL_VALUE}
        onValueChange={(next) => onChange(next === ALL_VALUE ? undefined : (next as ActivityClass))}
      >
        <SelectTrigger
          size="sm"
          className="w-40 cursor-pointer"
          aria-labelledby="activity-filter-label"
          data-testid="activity-filter"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL_VALUE}>{t("coordinator:activityFilterAll")}</SelectItem>
          {ACTIVITY_CLASSES.map((actionClass) => (
            <SelectItem key={actionClass} value={actionClass}>
              {t(CLASS_LABEL_KEY[actionClass])}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
