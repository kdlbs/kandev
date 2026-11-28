import type { Automation, AutomationTrigger } from "@/lib/types/automation";
import type { FormState } from "./automation-payload";
import { ConfigSection } from "./config-section";
import { supportsPersonalCredentialChoices } from "./automation-credential-eligibility";
import type { AutomationCredentialError } from "./automation-credential-error";

type UpdateField = <K extends keyof FormState>(key: K, value: FormState[K]) => void;

export function AutomationCredentialConfig({
  form,
  triggers,
  workspaceId,
  automation,
  onAutomationChange,
  updateField,
  credentialError,
  dirtyFields,
}: {
  form: FormState;
  triggers: AutomationTrigger[];
  workspaceId: string;
  automation: Automation | null;
  onAutomationChange: (automation: Automation) => void;
  updateField: UpdateField;
  credentialError: AutomationCredentialError | null;
  dirtyFields: {
    agentProfileId: boolean;
    executorProfileId: boolean;
    repositorySelections: boolean;
  };
}) {
  if (form.taskMode === "managed_conversation") return null;
  const credentialChoicesEligible = supportsPersonalCredentialChoices(
    form.taskMode,
    form.continuationPolicy,
    triggers,
  );
  const canChangeCredentialChoices =
    credentialChoicesEligible && (automation?.can_manage_credential_choices ?? true);
  const updatePersonalCredentialBindings = (bindings: FormState["personalCredentialBindings"]) => {
    if (canChangeCredentialChoices) updateField("personalCredentialBindings", bindings);
  };
  return (
    <ConfigSection
      workspaceId={workspaceId}
      automation={automation}
      onAutomationChange={onAutomationChange}
      agentProfileId={form.agentProfileId}
      executorProfileId={form.executorProfileId}
      credentialChoicesEligible={credentialChoicesEligible}
      credentialError={credentialError}
      personalCredentialBindings={form.personalCredentialBindings}
      onPersonalCredentialBindingsChange={updatePersonalCredentialBindings}
      repositorySelections={form.repositorySelections}
      dirtyFields={dirtyFields}
      onAgentProfileChange={(value) => {
        updateField("agentProfileId", value);
        updatePersonalCredentialBindings(
          form.personalCredentialBindings.filter(
            (binding) => binding.profile_id !== form.agentProfileId,
          ),
        );
      }}
      onExecutorProfileChange={(value) => {
        updateField("executorProfileId", value);
        updatePersonalCredentialBindings(
          form.personalCredentialBindings.filter(
            (binding) => binding.profile_id !== form.executorProfileId,
          ),
        );
      }}
      onRepositoriesChange={(value) => {
        updateField("repositorySelections", value);
        updateField("repositoryMode", value.length > 0 ? "selected" : "none");
      }}
    />
  );
}
