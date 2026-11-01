"use client";

import { useTranslation } from "react-i18next";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { WalkthroughMobileStepPicker } from "./walkthrough-mobile-step-picker";
import type { TaskWalkthrough } from "@/lib/types/http";

export function WalkthroughStepPicker({
  walkthrough,
  activeStep,
  counts,
  onSelect,
}: {
  walkthrough: TaskWalkthrough;
  activeStep: number;
  counts: number[];
  onSelect: (index: number) => void;
}) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const options = walkthrough.steps.map((step, index) => {
    const title = step.title || t("diff:walkthroughUntitledStep", { number: index + 1 });
    const count = counts[index] ?? 0;
    return {
      title,
      comments: t("diff:walkthroughCommentCount", { count }),
      label: t("diff:walkthroughStepOption", {
        number: index + 1,
        title,
        count,
      }),
    };
  });
  if (isMobile || !isFinePointer) {
    return (
      <div className="mx-3 mt-2 min-w-0 shrink-0">
        <WalkthroughMobileStepPicker
          options={options}
          activeStep={activeStep}
          onSelect={onSelect}
        />
      </div>
    );
  }
  return (
    <div className="mx-3 mt-2 min-w-0 shrink-0">
      <Select value={String(activeStep)} onValueChange={(value) => onSelect(Number(value))}>
        <SelectTrigger
          aria-label={t("diff:walkthroughSelectStep")}
          data-testid="walkthrough-step-picker"
          className="h-7 w-full min-w-0 text-sm max-md:h-11 pointer-coarse:h-11"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="max-w-[calc(100vw-2rem)]">
          {options.map((option, index) => (
            <SelectItem
              key={index}
              value={String(index)}
              className="pr-7 text-sm max-md:min-h-11 pointer-coarse:min-h-11 [overflow-wrap:anywhere]"
            >
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
