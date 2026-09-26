# TYPO-007: Remove agent profile compact typography drift

- Priority: P1
- Status: In progress
- Scope: Agent profile settings and any shared compact profile form consumers
- Related: TYPO-004, TYPO-005, TYPO-006

## Finding

The agent profile form has a compact variant that changes the typography of the same setting concepts instead of only changing layout density. Compact permission descriptions become 10px with tight leading, while the default form uses 12px. Labels also change through conditional classes.

## Evidence

- `apps/web/components/settings/profile-form-fields.tsx:49` defines the default muted text role as `text-xs text-muted-foreground`.
- `apps/web/components/settings/profile-form-fields.tsx:124-145` changes label styling based on `compact`.
- `apps/web/components/settings/profile-form-fields.tsx:141-145` uses `text-[10px] text-muted-foreground leading-tight` for compact descriptions.
- `apps/web/components/settings/profile-form-fields.tsx:186-194` and `apps/web/components/settings/profile-form-fields.tsx:237-245` render the same passthrough setting with different label and description scales between compact and default paths.
- `apps/web/components/settings/profile-model-fields.tsx:195-232` and `apps/web/components/settings/profile-model-fields.tsx:282-291` add more local label/helper choices around the profile model controls.

## User impact

Agent profile settings can change perceived information hierarchy when a compact mode is used. Ten-pixel explanatory copy is difficult to read, especially on a phone or when translated text wraps.

## Proposed direction

Keep compact mode focused on spacing, grid, and control density. Reuse the same field-label and helper roles unless a smaller role is explicitly approved for non-essential metadata. If a compact helper must exist, make it a named semantic variant with a minimum readable size and a mobile rule.

## Verification

- Identify every settings and onboarding consumer of `ProfileFormFields` and compare default versus compact rendering.
- Add a focused component test for the chosen role classes.
- Run agent profile desktop and mobile flows with long descriptions and pseudo-locale copy.

## Progress

- 2026-08-16: Opened from the settings source audit.
