"use client";

import { Button } from "@kandev/ui/button";
import { IconSparkles } from "@tabler/icons-react";
import { PanelHeaderBarSplit } from "./panel-primitives";

type NotePanelHeaderProps = {
  canEnhance: boolean;
  isEnhancing: boolean;
  onEnhance: () => void;
};

export function TaskNotePanelHeader({ canEnhance, isEnhancing, onEnhance }: NotePanelHeaderProps) {
  return (
    <PanelHeaderBarSplit
      left={null}
      right={
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="cursor-pointer"
          disabled={!canEnhance || isEnhancing}
          onClick={onEnhance}
          data-testid="enhance-note-with-ai-button"
        >
          <IconSparkles className="mr-1.5 h-3.5 w-3.5" />
          Enhance note with AI
        </Button>
      }
    />
  );
}
