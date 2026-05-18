"use client";

import { Button } from "@kandev/ui/button";
import { IconArrowLeft, IconArrowRight, IconRocket } from "@tabler/icons-react";

const STEP_COUNT = 4;

type WizardFooterProps = {
  step: number;
  canAdvance: boolean;
  submitting: boolean;
  onBack: () => void;
  onNext: () => void;
  onSkip: () => void;
  onSubmit: () => void;
};

export function WizardFooter({
  step,
  canAdvance,
  submitting,
  onBack,
  onNext,
  onSkip,
  onSubmit,
}: WizardFooterProps) {
  const isLast = step === STEP_COUNT - 1;
  const isTaskStep = step === 2;

  return (
    <div className="flex items-center justify-between mt-8">
      <div className="flex items-center gap-2">
        {step > 0 && (
          <Button variant="ghost" onClick={onBack} className="cursor-pointer">
            <IconArrowLeft className="h-4 w-4 mr-1" />
            Back
          </Button>
        )}
      </div>
      <div className="flex items-center gap-2">
        {isTaskStep && (
          <Button variant="ghost" onClick={onSkip} className="cursor-pointer">
            Skip
          </Button>
        )}
        {isLast ? (
          <Button onClick={onSubmit} disabled={submitting} className="cursor-pointer">
            <IconRocket className="h-4 w-4 mr-1" />
            {submitting ? "Creating..." : "Create & Launch"}
          </Button>
        ) : (
          <Button onClick={onNext} disabled={!canAdvance} className="cursor-pointer">
            Next
            <IconArrowRight className="h-4 w-4 ml-1" />
          </Button>
        )}
      </div>
    </div>
  );
}
