'use client';

import { Fragment, useCallback, useEffect, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from '@kandev/ui/dialog';
import { Button } from '@kandev/ui/button';
import {
  IconArrowRight,
  IconArrowLeft,
  IconCheck,
  IconFolder,
  IconFolders,
  IconBrandDocker,
  IconX,
  IconLoader2,
  IconChevronDown,
  IconCommand,
  IconSearch,
  IconHome,
  IconGitCommit,
  IconTerminal2,
  IconArrowDown,
} from '@tabler/icons-react';
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from '@kandev/ui/collapsible';
import { Kbd } from '@kandev/ui/kbd';
import { AgentLogo } from '@/components/agent-logo';
import { ProfileFormFields, type ProfileFormData } from '@/components/settings/profile-form-fields';
import { profileToPermissionsMap, permissionsToProfilePatch } from '@/lib/agent-permissions';
import { listAvailableAgents, listWorkflowTemplates } from '@/lib/api';
import { listAgentsAction, updateAgentProfileAction } from '@/app/actions/agents';
import type { AvailableAgent, WorkflowTemplate, AgentProfile } from '@/lib/types/http';

interface OnboardingDialogProps {
  open: boolean;
  onComplete: () => void;
}

type AgentSetting = {
  profileId: string;
  formData: ProfileFormData;
  dirty: boolean;
};

const TOTAL_STEPS = 4;

const RUNTIMES = [
  {
    name: 'Local',
    description: 'Run agents directly on your machine with full access to your local filesystem.',
    icon: IconFolder,
  },
  {
    name: 'Git Worktree',
    description: 'Isolated branch environment under a worktree root for parallel work.',
    icon: IconFolders,
  },
  {
    name: 'Docker',
    description: 'Containerized execution for full isolation and reproducibility.',
    icon: IconBrandDocker,
  },
];

const STEP_TITLES = ['AI Agents', 'Environments', 'Agentic Workflows', 'Command Panel'];
const STEP_DESCRIPTIONS = [
  'These AI coding agents were discovered on your system.',
  'Agents can run in different runtime environments.',
  'Workflows define the steps and automation for your tasks.',
  'Quick access to actions from anywhere with a keyboard shortcut.',
];

function buildAgentSettings(avail: AvailableAgent[], saved: { name: string; profiles?: { id: string; name: string; model?: string | null; cli_passthrough?: boolean | null }[] }[]): Record<string, AgentSetting> {
  const settings: Record<string, AgentSetting> = {};
  for (const aa of avail) {
    const dbAgent = saved.find((a) => a.name === aa.name);
    const profile = dbAgent?.profiles?.[0];
    if (profile) {
      const perms = profileToPermissionsMap(profile as Partial<Pick<AgentProfile, 'auto_approve' | 'dangerously_skip_permissions' | 'allow_indexing'>>, aa.permission_settings ?? {});
      settings[aa.name] = { profileId: profile.id, formData: { name: profile.name, model: profile.model || aa.model_config.default_model, cli_passthrough: profile.cli_passthrough ?? false, ...perms }, dirty: false };
    }
  }
  return settings;
}

type OnboardingFooterProps = { step: number; onSkip: () => void; onBack: () => void; onNext: () => void; onGetStarted: () => void };

function OnboardingFooter({ step, onSkip, onBack, onNext, onGetStarted }: OnboardingFooterProps) {
  return (
    <DialogFooter>
      <div className="flex w-full items-center justify-between">
        <Button variant="ghost" size="sm" onClick={onSkip} className="cursor-pointer"><IconX className="mr-1.5 h-3.5 w-3.5" />Skip</Button>
        <div className="flex gap-2">
          {step > 0 && <Button variant="outline" onClick={onBack} className="cursor-pointer"><IconArrowLeft className="mr-1.5 h-4 w-4" />Back</Button>}
          {step < TOTAL_STEPS - 1
            ? <Button onClick={onNext} className="cursor-pointer">Next<IconArrowRight className="ml-1.5 h-4 w-4" /></Button>
            : <Button onClick={onGetStarted} className="cursor-pointer"><IconCheck className="mr-1.5 h-4 w-4" />Get Started</Button>
          }
        </div>
      </div>
    </DialogFooter>
  );
}

export function OnboardingDialog({ open, onComplete }: OnboardingDialogProps) {
  const [step, setStep] = useState(0);
  const [availableAgents, setAvailableAgents] = useState<AvailableAgent[]>([]);
  const [agentSettings, setAgentSettings] = useState<Record<string, AgentSetting>>({});
  const [templates, setTemplates] = useState<WorkflowTemplate[]>([]);
  const [loadingAgents, setLoadingAgents] = useState(true);
  const [loadingTemplates, setLoadingTemplates] = useState(true);
  const [prevOpen, setPrevOpen] = useState(false);

  if (open && !prevOpen) { setPrevOpen(true); setLoadingAgents(true); setLoadingTemplates(true); }
  else if (!open && prevOpen) { setPrevOpen(false); }

  useEffect(() => {
    if (!open) return;
    Promise.all([listAvailableAgents({ cache: 'no-store' }), listAgentsAction()])
      .then(([availRes, savedRes]) => { setAvailableAgents(availRes.agents ?? []); setAgentSettings(buildAgentSettings(availRes.agents ?? [], savedRes.agents ?? [])); })
      .catch(() => { }).finally(() => setLoadingAgents(false));
    listWorkflowTemplates().then((res) => setTemplates(res.templates ?? [])).catch(() => { }).finally(() => setLoadingTemplates(false));
  }, [open]);

  const saveAgentSettings = useCallback(async () => {
    await Promise.all(Object.values(agentSettings).filter((s) => s.dirty).map((s) => updateAgentProfileAction(s.profileId, { model: s.formData.model, ...permissionsToProfilePatch(s.formData), cli_passthrough: s.formData.cli_passthrough })));
  }, [agentSettings]);

  const handleSkip = () => { onComplete(); setStep(0); };
  const handleNext = async () => { if (step === 0) await saveAgentSettings(); if (step < TOTAL_STEPS - 1) setStep(step + 1); };
  const handleBack = () => { if (step > 0) setStep(step - 1); };
  const handleGetStarted = async () => { await saveAgentSettings(); onComplete(); setStep(0); };
  const updateSetting = (agentName: string, formPatch: Partial<ProfileFormData>) => {
    setAgentSettings((prev) => ({ ...prev, [agentName]: { ...prev[agentName], formData: { ...prev[agentName].formData, ...formPatch }, dirty: true } }));
  };

  return (
    <Dialog open={open} onOpenChange={() => { }}>
      <DialogContent className="sm:max-w-[540px]" showCloseButton={false}>
        <DialogHeader>
          <DialogTitle className="text-center text-2xl">{STEP_TITLES[step]}</DialogTitle>
          <DialogDescription className="text-center">{STEP_DESCRIPTIONS[step]}</DialogDescription>
        </DialogHeader>
        <div className="py-4 min-h-[220px]">
          {step === 0 && <StepAgents availableAgents={availableAgents} agentSettings={agentSettings} loading={loadingAgents} onUpdateSetting={updateSetting} />}
          {step === 1 && <StepEnvironments />}
          {step === 2 && <StepWorkflows templates={templates} loading={loadingTemplates} />}
          {step === 3 && <StepCommandPanel />}
        </div>
        <div className="flex justify-center gap-1.5 pb-2">
          {Array.from({ length: TOTAL_STEPS }).map((_, i) => (<div key={i} className={`h-1.5 rounded-full transition-all ${i === step ? 'w-6 bg-primary' : 'w-1.5 bg-muted-foreground/30'}`} />))}
        </div>
        <OnboardingFooter step={step} onSkip={handleSkip} onBack={handleBack} onNext={handleNext} onGetStarted={handleGetStarted} />
      </DialogContent>
    </Dialog>
  );
}

function StepAgents({
  availableAgents,
  agentSettings,
  loading,
  onUpdateSetting,
}: {
  availableAgents: AvailableAgent[];
  agentSettings: Record<string, AgentSetting>;
  loading: boolean;
  onUpdateSetting: (agentName: string, formPatch: Partial<ProfileFormData>) => void;
}) {
  const [openAgent, setOpenAgent] = useState<string | null>(null);

  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-3 text-sm text-muted-foreground">
        <IconLoader2 className="h-6 w-6 animate-spin" />
        Discovering agents...
      </div>
    );
  }

  const agents = availableAgents.filter((a) => a.available);

  return (
    <div className="space-y-3">
      <div className="grid gap-2 max-h-[320px] overflow-y-auto pr-1">
        {agents.map((agent) => {
          const settings = agentSettings[agent.name];
          const currentModel = settings?.formData.model || agent.model_config.default_model;
          const modelName =
            agent.model_config.available_models.find((m) => m.id === currentModel)?.name ??
            currentModel;

          return (
            <Collapsible
              key={agent.name}
              open={openAgent === agent.name}
              onOpenChange={(isOpen) => setOpenAgent(isOpen ? agent.name : null)}
            >
              <CollapsibleTrigger asChild>
                <button
                  type="button"
                  className="flex w-full items-center gap-3 rounded-lg border p-2 text-left cursor-pointer hover:bg-muted/50 transition-colors group"
                >
                  <AgentLogo agentName={agent.name} size={28} />
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium">{agent.display_name}</p>
                  </div>
                  <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-medium truncate max-w-[120px]">
                    {modelName}
                  </span>
                  <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
                    <IconCheck className="h-3.5 w-3.5" />
                    Installed
                  </span>
                  <IconChevronDown className="h-4 w-4 text-muted-foreground transition-transform group-data-[state=open]:rotate-180" />
                </button>
              </CollapsibleTrigger>
              <CollapsibleContent>
                <div className="border border-t-0 rounded-b-lg px-3 pb-3 pt-2">
                  {settings && (
                    <ProfileFormFields
                      variant="compact"
                      hideNameField
                      profile={settings.formData}
                      onChange={(patch) => onUpdateSetting(agent.name, patch)}
                      modelConfig={agent.model_config}
                      permissionSettings={agent.permission_settings ?? {}}
                      passthroughConfig={agent.passthrough_config ?? null}
                      agentName={agent.name}
                    />
                  )}
                </div>
              </CollapsibleContent>
            </Collapsible>
          );
        })}
      </div>
      <p className="text-xs text-muted-foreground">
        Expand an agent to configure its model and permissions. Changes are saved when you proceed.
        You can also add custom TUI agents later in Settings &gt; Agents.
      </p>
    </div>
  );
}

function StepEnvironments() {
  return (
    <div className="space-y-3">
      <div className="grid gap-2">
        {RUNTIMES.map((runtime) => {
          const Icon = runtime.icon;
          return (
            <div
              key={runtime.name}
              className="flex items-start gap-3 rounded-lg border p-3"
            >
              <div className="h-8 w-8 rounded-md bg-muted flex items-center justify-center flex-shrink-0">
                <Icon className="h-4.5 w-4.5 text-muted-foreground" />
              </div>
              <div className="min-w-0">
                <p className="text-sm font-medium">{runtime.name}</p>
                <p className="text-xs text-muted-foreground">{runtime.description}</p>
              </div>
            </div>
          );
        })}
      </div>
      <p className="text-xs text-muted-foreground">
        Configure runtime environments in Settings to control where agents execute.
      </p>
    </div>
  );
}

function StepWorkflows({ templates, loading }: { templates: WorkflowTemplate[]; loading: boolean }) {
  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-3 text-sm text-muted-foreground">
        <IconLoader2 className="h-6 w-6 animate-spin" />
        Loading workflow templates...
      </div>
    );
  }

  const defaultTemplate = templates.find((t) => t.id === 'simple');
  const otherTemplates = templates.filter((t) => t.id !== 'simple');

  return (
    <div className="space-y-3">
      <div className="grid gap-2 max-h-[320px] overflow-y-auto pr-1">
        {defaultTemplate && (
          <TemplateCard template={defaultTemplate} isDefault />
        )}
        {otherTemplates.length > 0 && (
          <>
            <p className="text-xs text-muted-foreground mt-1">Available templates</p>
            {otherTemplates.map((template) => (
              <TemplateCard key={template.id} template={template} />
            ))}
          </>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        Workflows control the steps, automation, and agent behavior for your tasks. You can add
        more workflows from Settings.
      </p>
    </div>
  );
}

const COMMAND_PANEL_PREVIEW_ITEMS = [
  { icon: IconSearch, label: 'Search Tasks', trailing: '→' },
  { icon: IconHome, label: 'Go to Home' },
  { icon: IconGitCommit, label: 'Commit Changes' },
  { icon: IconArrowDown, label: 'Pull' },
  { icon: IconTerminal2, label: 'Add Terminal Panel' },
];

function StepCommandPanel() {
  return (
    <div className="space-y-4">
      {/* Mock command panel preview */}
      <div className="rounded-lg border bg-card overflow-hidden">
        {/* Search input */}
        <div className="flex items-center gap-2 px-3 py-2 border-b">
          <IconSearch className="h-3.5 w-3.5 text-muted-foreground/50" />
          <span className="text-xs text-muted-foreground/50">Type a command...</span>
        </div>
        {/* Sample commands */}
        <div className="py-1">
          {COMMAND_PANEL_PREVIEW_ITEMS.map((item) => {
            const Icon = item.icon;
            return (
              <div
                key={item.label}
                className="flex items-center gap-3 px-3 py-1.5 text-sm first:bg-muted/50"
              >
                <Icon className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="flex-1 text-xs">{item.label}</span>
                {item.trailing && (
                  <span className="text-xs text-muted-foreground">{item.trailing}</span>
                )}
              </div>
            );
          })}
        </div>
        {/* Footer */}
        <div className="flex items-center gap-3 px-3 py-1.5 border-t text-muted-foreground">
          <span className="inline-flex items-center gap-1">
            <Kbd>↑</Kbd>
            <Kbd>↓</Kbd>
            <span className="text-[0.6rem]">Navigate</span>
          </span>
          <span className="inline-flex items-center gap-1">
            <Kbd>↵</Kbd>
            <span className="text-[0.6rem]">Select</span>
          </span>
          <span className="inline-flex items-center gap-1">
            <Kbd>esc</Kbd>
            <span className="text-[0.6rem]">Close</span>
          </span>
        </div>
      </div>

      {/* Shortcut hint */}
      <div className="flex items-center justify-center gap-2 text-sm text-muted-foreground">
        <span>Press</span>
        <span className="inline-flex items-center gap-0.5">
          <Kbd><IconCommand className="size-3" /></Kbd>
          <Kbd>K</Kbd>
        </span>
        <span>to open it anytime</span>
      </div>

      <p className="text-xs text-muted-foreground">
        Navigate between pages, search tasks, trigger git operations, and manage panels — all
        without leaving the keyboard. Context-aware commands appear based on the active page.
      </p>
    </div>
  );
}

function TemplateCard({ template, isDefault }: { template: WorkflowTemplate; isDefault?: boolean }) {
  const steps = (template.default_steps ?? [])
    .slice()
    .sort((a, b) => a.position - b.position);

  return (
    <div
      className={`rounded-lg border p-3 ${isDefault ? 'border-primary/50 bg-primary/5' : 'opacity-60'}`}
    >
      <div className="flex items-center gap-2">
        <p className="text-sm font-medium">{template.name}</p>
        {isDefault && (
          <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
            <IconCheck className="h-3.5 w-3.5" />
            Default
          </span>
        )}
      </div>
      {template.description && (
        <p className="text-xs text-muted-foreground mt-0.5">{template.description}</p>
      )}
      {steps.length > 0 && (
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground whitespace-nowrap mt-2">
          {steps.map((s, i) => (
            <Fragment key={s.name}>
              {i > 0 && <span className="text-muted-foreground/40">→</span>}
              <span className="flex items-center gap-1">
                <span
                  className="h-1.5 w-1.5 rounded-full shrink-0"
                  style={{ backgroundColor: s.color || 'hsl(var(--muted-foreground))' }}
                />
                {s.name}
              </span>
            </Fragment>
          ))}
        </div>
      )}
    </div>
  );
}
