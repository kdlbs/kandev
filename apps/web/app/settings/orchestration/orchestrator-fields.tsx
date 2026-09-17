import { useTranslation } from "react-i18next";
import { Textarea } from "@kandev/ui/textarea";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import Link from "@/components/routing/app-link";
import { PersonaExecutorField } from "@/components/shared/persona-executor-field";
import { selectedExecutor } from "@/lib/api/domains/orchestration-api";
import type {
  OrchestratorConfiguration,
  OrchestratorRole,
  OrchestrationProfile,
} from "@/lib/api/domains/orchestration-api";
type OrchestratorFieldsProps = {
  value: OrchestratorConfiguration;
  onChange: (value: OrchestratorConfiguration) => void;
  roles: OrchestratorRole[];
  profiles: OrchestrationProfile[];
};
export function OrchestratorFields({ value, onChange, roles, profiles }: OrchestratorFieldsProps) {
  const { t } = useTranslation();
  const patch = (v: Partial<OrchestratorConfiguration>) => onChange({ ...value, ...v });
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>{t("orchestration:role")}</Label>
        <Select value={value.role_id} onValueChange={(id) => patch({ role_id: id })}>
          <SelectTrigger data-testid="orchestrator-role">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {roles.map((r) => (
              <SelectItem value={r.id} key={r.id}>
                {r.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">{t("orchestration:roleAssignmentHint")}</p>
      </div>
      <div className="space-y-2">
        <Label>{t("orchestration:orchestratorProfile")}</Label>
        <Select value={value.profile_id} onValueChange={(id) => patch({ profile_id: id })}>
          <SelectTrigger data-testid="orchestrator-profile">
            <SelectValue placeholder={t("orchestration:chooseExecutionProfile")} />
          </SelectTrigger>
          <SelectContent>
            {profiles.map((p) => (
              <SelectItem value={p.id} key={p.id}>
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">
          {t("orchestration:orchestratorProfileHint")}
        </p>
        <Link className="text-sm underline" href="/settings/agents">
          {t("orchestration:manageExecutionProfiles")}
        </Link>
      </div>
      <PersonaExecutorField
        value={selectedExecutor(value.executor_preference)}
        onChange={(id, type) =>
          patch({ executor_preference: JSON.stringify({ type, executor_profile_id: id }) })
        }
      />
      <div className="space-y-2">
        <Label htmlFor="orchestrator-context">{t("orchestration:orchestratorContext")}</Label>
        <Textarea
          id="orchestrator-context"
          value={value.context}
          maxLength={2000}
          onChange={(e) => patch({ context: e.target.value })}
        />
      </div>
    </div>
  );
}
