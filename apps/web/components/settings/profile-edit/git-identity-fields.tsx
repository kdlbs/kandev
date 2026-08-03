import { Badge } from "@kandev/ui/badge";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { useTranslation } from "react-i18next";

export type GitIdentityMode = "local" | "override";
export type GitIdentityState = {
  userName: string;
  userEmail: string;
  detected: boolean;
};

type GitIdentityFieldsProps = {
  mode: GitIdentityMode;
  baselineMode: GitIdentityMode;
  onModeChange: (mode: GitIdentityMode) => void;
  gitUserName: string;
  gitUserEmail: string;
  baselineGitUserName: string;
  baselineGitUserEmail: string;
  onGitUserNameChange: (value: string) => void;
  onGitUserEmailChange: (value: string) => void;
  localGitIdentity: GitIdentityState;
};

const RADIO_LABEL =
  "flex w-full items-start gap-3 rounded-md border p-3 text-left cursor-pointer transition-colors";
const RADIO_ITEM = "mt-0.5 border border-muted-foreground/80 data-[state=checked]:border-primary";

export function GitIdentityAccordionItem(props: GitIdentityFieldsProps) {
  const { t } = useTranslation();
  const isLocal = props.mode === "local" && props.localGitIdentity.detected;
  const modeDirty = props.mode !== props.baselineMode;
  const nameDirty = props.mode === "override" && props.gitUserName !== props.baselineGitUserName;
  const emailDirty = props.mode === "override" && props.gitUserEmail !== props.baselineGitUserEmail;
  const description = props.localGitIdentity.detected
    ? `${props.localGitIdentity.userName} <${props.localGitIdentity.userEmail}>`
    : t("executors:localGitIdentityNotDetected");
  const badgeLabel = t(gitIdentityBadgeLabelKey(props.mode, isLocal));

  return (
    <AccordionItem value="git_identity" data-settings-dirty={modeDirty || nameDirty || emailDirty}>
      <AccordionTrigger>
        <div className="flex flex-1 items-center gap-2">
          <span className="text-sm font-medium">{t("executors:gitIdentity")}</span>
          <Badge
            variant={isLocal ? "default" : "secondary"}
            className={isLocal ? "bg-green-600 px-1.5 py-0 text-[10px]" : "px-1.5 py-0 text-[10px]"}
          >
            {badgeLabel}
          </Badge>
        </div>
      </AccordionTrigger>
      <AccordionContent className="h-auto">
        <div className="space-y-3 text-sm">
          <p className="text-xs text-muted-foreground">
            {t("executors:usedByRemoteExecutorsForCommit")}
          </p>
          <RadioGroup
            value={props.mode}
            onValueChange={(value) => props.onModeChange(value as GitIdentityMode)}
            className="gap-2"
            data-settings-dirty={modeDirty}
          >
            <IdentityModeOption
              value="local"
              selected={props.mode === "local"}
              baselineSelected={props.baselineMode === "local"}
              disabled={!props.localGitIdentity.detected}
              title={t("executors:useLocalGitConfig")}
              description={description}
            />
            <IdentityModeOption
              value="override"
              selected={props.mode === "override"}
              baselineSelected={props.baselineMode === "override"}
              title={t("executors:overrideIdentity")}
              description={t("executors:setACustomNameAndEmail")}
            />
          </RadioGroup>
          {props.mode === "override" && <OverrideIdentityFields {...props} />}
        </div>
      </AccordionContent>
    </AccordionItem>
  );
}

// Returns a catalog key, not copy: module scope, so a `t()` here would freeze
// the badge at the boot locale.
function gitIdentityBadgeLabelKey(mode: GitIdentityMode, isLocal: boolean): string {
  if (isLocal) return "executors:gitIdentityAutoDetect";
  return mode === "local" ? "executors:notConfigured" : "executors:gitIdentityCustom";
}

function IdentityModeOption({
  value,
  selected,
  baselineSelected,
  disabled,
  title,
  description,
}: {
  value: GitIdentityMode;
  selected: boolean;
  baselineSelected: boolean;
  disabled?: boolean;
  title: string;
  description: string;
}) {
  return (
    <label
      className={`${RADIO_LABEL} ${selected ? "border-primary bg-primary/5" : "border-border"}`}
      data-settings-dirty={selected !== baselineSelected}
    >
      <RadioGroupItem value={value} disabled={disabled} className={RADIO_ITEM} />
      <div className="flex flex-col gap-0.5">
        <span className="text-sm font-medium">{title}</span>
        <span className="text-xs text-muted-foreground">{description}</span>
      </div>
    </label>
  );
}

function OverrideIdentityFields(props: GitIdentityFieldsProps) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor="remote-git-user-name">{t("executors:gitUserName")}</Label>
        <Input
          id="remote-git-user-name"
          value={props.gitUserName}
          onChange={(event) => props.onGitUserNameChange(event.target.value)}
          placeholder={t("executors:janeDeveloper")}
          data-settings-dirty={props.gitUserName !== props.baselineGitUserName}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="remote-git-user-email">{t("executors:gitUserEmail")}</Label>
        <Input
          id="remote-git-user-email"
          value={props.gitUserEmail}
          onChange={(event) => props.onGitUserEmailChange(event.target.value)}
          placeholder="jane@example.com"
          data-settings-dirty={props.gitUserEmail !== props.baselineGitUserEmail}
        />
      </div>
    </div>
  );
}
