import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";

export function TasksWorkspaceError({ missing }: { missing: boolean }) {
  const { t } = useTranslation();
  const router = useRouter();
  const retry = useAppStore((state) => state.requestWorkspaceContextRefresh);
  return (
    <div className="flex h-full min-h-0 flex-col items-center justify-center gap-4 p-4">
      <p role="alert" className="text-sm text-muted-foreground">
        {t(missing ? "workspaces:workspaceNotFound" : "common:failedToLoadRoute")}
      </p>
      <div className="flex gap-2">
        {!missing && (
          <Button
            onClick={() => retry()}
            className={controlSizingClassName("standard", "cursor-pointer")}
          >
            {t("task:retry")}
          </Button>
        )}
        <Button
          variant="outline"
          onClick={() => router.push("/")}
          className={controlSizingClassName("standard", "cursor-pointer")}
        >
          {t("sidebar:home")}
        </Button>
      </div>
    </div>
  );
}
