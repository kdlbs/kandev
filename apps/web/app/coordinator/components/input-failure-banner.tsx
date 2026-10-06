import { useTranslation } from "react-i18next";
import { Alert, AlertAction, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import type { CoordinatorInputStatus } from "@/app/coordinator/use-coordinator-attention";
import { formatLocalTime } from "@/lib/coordinator/format";

const LINE_KEYS: Record<CoordinatorInputStatus["kind"], { withTime: string; firstLoad: string }> = {
  tasks: {
    withTime: "coordinator:bannerTasksFailed",
    firstLoad: "coordinator:bannerTasksFailedFirstLoad",
  },
  stalls: {
    withTime: "coordinator:bannerStallsFailed",
    firstLoad: "coordinator:bannerStallsFailedFirstLoad",
  },
  proposals: {
    withTime: "coordinator:bannerProposalsFailed",
    firstLoad: "coordinator:bannerProposalsFailedFirstLoad",
  },
};

export type InputFailureBannerProps = {
  inputs: CoordinatorInputStatus[];
  retry: () => void;
};

/**
 * One banner, one line per failed input, in the order tasks/stalls/proposals,
 * each with its own load time or the timeless first-load sentence
 * (AC-COORDINATOR-NEEDS-YOU-007.3, .007.5). Renders nothing when no input has
 * failed.
 */
export function InputFailureBanner({ inputs, retry }: InputFailureBannerProps) {
  const { t } = useTranslation();
  const failed = inputs.filter((input) => input.error);
  if (failed.length === 0) return null;

  return (
    <Alert variant="destructive" data-testid="coordinator-input-failure-banner">
      <AlertDescription>
        {failed.map((input) => {
          const keys = LINE_KEYS[input.kind];
          const text =
            input.loadedAt === undefined
              ? t(keys.firstLoad)
              : t(keys.withTime, { time: formatLocalTime(input.loadedAt) });
          return <p key={input.kind}>{text}</p>;
        })}
      </AlertDescription>
      <AlertAction>
        <Button variant="outline" size="sm" onClick={retry}>
          {t("coordinator:tryAgain")}
        </Button>
      </AlertAction>
    </Alert>
  );
}
