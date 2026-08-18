/**
 * Seeds for the first task the onboarding wizard creates.
 *
 * Deliberately NOT localized. The title is persisted as the task's own title and
 * the description is sent verbatim to the coordinator agent as its prompt —
 * translating either would change stored data and the instructions the model
 * receives, which is a bug rather than a missing translation.
 */
// i18n-exempt: persisted as the task's own title. See the file header.
export const DEFAULT_ONBOARDING_TASK_TITLE = "Setup Workspace";

// i18n-exempt: sent verbatim to the coordinator agent as its prompt. See the file header.
export const DEFAULT_ONBOARDING_TASK_DESCRIPTION = `Investigate the project in https://github.com/org/repo. Replace this URL with each repository that should be managed in this workspace.

As the CEO agent:
- Inspect the repository or repositories and summarize the product, architecture, tech stack, and current risks.
- Create one project per repository, linking each project to its repository and capturing the main goals.
- Create the agent team needed to own the work, such as engineering lead, frontend, backend, QA, DevOps, product/research, or documentation agents as appropriate.
- Give each agent clear responsibilities, permissions, and operating guidance.
- Draft a proposed plan for the next setup steps, including repo discovery, build/test verification, initial improvements, and which tasks or subtasks should be created.
- Wait for the human to approve the proposed plan before creating follow-up tasks or making risky changes.
- Note assumptions, missing access, and questions for the human.`;
