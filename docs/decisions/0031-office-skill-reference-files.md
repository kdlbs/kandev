# 0031: Office Skill Reference Files

**Status:** accepted
**Date:** 2026-07-06
**Area:** backend

## Context

Office system skills are bundled under `apps/backend/internal/office/configloader/skills/` and synced into `office_skills`. The original system-skill parser stored only the markdown body after YAML frontmatter, so the runtime deployer generated placeholder frontmatter such as `description: kandev-approvals`. The bundled skill set also had overlapping small skills that would be better expressed as concise entry skills plus reference files, but the runtime deployer only wrote `SKILL.md`.

## Decision

Bundled system skills preserve the full `SKILL.md` content, including YAML frontmatter, when synced into `office_skills`. The system sync also records non-`SKILL.md` bundled files in `file_inventory` with their content, and the runtime skill deployer materializes those files next to `SKILL.md` for local, Docker, and Sprites executors.

The Office bundle now uses progressive disclosure for broader system skills such as `kandev-team-admin`, `kandev-task-ops`, and `kandev-config-sync`, with detailed guidance in `references/`.

## Consequences

Skill descriptions used for agent discovery remain meaningful at runtime instead of being replaced by slug-only generated frontmatter. Bundled skills can be consolidated without losing detailed instructions, reducing the number of attached skills while keeping context load small. The runtime deployer now validates support-file paths before writing them, so inventory entries cannot escape the deployed skill directory.

Existing body-only system rows are refreshed during system sync even when their historical `content_hash` already matches the full bundled file hash.

## Alternatives Considered

Keeping all Office workflows as separate skills avoided runtime changes, but continued to produce a noisy skill list and made discovery less precise. Storing reference text inline in `SKILL.md` avoided support-file materialization, but would increase prompt/context load for every skill discovery. Renaming only the descriptions without preserving frontmatter would not fix the deployed placeholder metadata.
