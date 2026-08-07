import { Button } from "@kandev/ui/button";

import type { AzureDevOpsBrowseMode } from "./azure-devops-filters";

type AzureDevOpsModeTabsProps = {
  mode: AzureDevOpsBrowseMode;
  onModeChange: (mode: AzureDevOpsBrowseMode) => void;
};

const modes: Array<{ mode: AzureDevOpsBrowseMode; label: string; testId: string }> = [
  { mode: "board", label: "Board", testId: "azure-devops-board-mode" },
  { mode: "work-items", label: "Work items", testId: "azure-devops-work-items-mode" },
  { mode: "pull-requests", label: "Pull requests", testId: "azure-devops-pull-requests-mode" },
];

export function AzureDevOpsModeTabs({ mode, onModeChange }: AzureDevOpsModeTabsProps) {
  return (
    <div className="flex items-center gap-1 border-b px-4 py-2">
      {modes.map((option) => (
        <Button
          key={option.mode}
          type="button"
          size="sm"
          variant={mode === option.mode ? "default" : "ghost"}
          className="cursor-pointer"
          onClick={() => onModeChange(option.mode)}
          data-testid={option.testId}
        >
          {option.label}
        </Button>
      ))}
    </div>
  );
}
