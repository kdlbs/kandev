import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

import type { AzureDevOpsBrowseMode } from "./azure-devops-filters";

type AzureDevOpsModeTabsProps = {
  mode: AzureDevOpsBrowseMode;
  onModeChange: (mode: AzureDevOpsBrowseMode) => void;
};

const modes: Array<{ mode: AzureDevOpsBrowseMode; labelKey: string; testId: string }> = [
  { mode: "board", labelKey: "azuredevops:board", testId: "azure-devops-board-mode" },
  { mode: "work-items", labelKey: "azuredevops:workItems", testId: "azure-devops-work-items-mode" },
  {
    mode: "pull-requests",
    labelKey: "azuredevops:pullRequests",
    testId: "azure-devops-pull-requests-mode",
  },
];

export function AzureDevOpsModeTabs({ mode, onModeChange }: AzureDevOpsModeTabsProps) {
  const { t } = useTranslation();
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
          {t(option.labelKey)}
        </Button>
      ))}
    </div>
  );
}
