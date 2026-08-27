---
id: "05-high-traffic-review"
title: "Human review high-traffic Traditional namespaces"
status: done
wave: 3
depends_on:
  - "03-generate-web-catalogs"
plan: "plan.md"
spec: "../../specs/platform/requirements/traditional-chinese-locales.md"
---

# Task 05: Human review high-traffic Traditional namespaces

## Intent

Mechanical conversion will miss nuance. Review and hand-edit the highest-traffic
namespaces so Taiwan and Hong Kong product vocabulary matches the glossary and
reads naturally in chrome users see first.

## Acceptance

- Reviewed (and fixed where needed) at least:
  `common`, `sidebar`, `settings`, `task`, `tasks`, `auth`, `github`,
  `workspaces`, `system` for both `zh-tw` and `zh-hk`.
- Glossary divergences are correct in those files (soft/hard TW vs HK pairs
  where the English concept appears).
- No type-to-confirm tokens or brands altered; interpolation/`Trans` indices
  intact (`i18n:check` still passes).
- Brief review notes appended to this task's Results (what was fixed, residual
  follow-ups).

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:parity
# Manual: activate zh-tw / zh-hk in dev and scan Settings + task chrome
```

## Files likely touched

- `apps/web/src/locales/zh-tw/{common,sidebar,settings,task,tasks,auth,github,workspaces,system}.json`
- `apps/web/src/locales/zh-hk/{common,sidebar,settings,task,tasks,auth,github,workspaces,system}.json`
- Possibly `docs/plans/traditional-chinese-locales/glossary.md` if new terms found

## Dependencies

- Task 03 generated catalogs.

## Parallelism

Sequential (shared catalog edits; review judgment).

## Inputs

- [glossary](glossary.md)
- Spec high-traffic review note in Out of scope / What

## Output contract

- Hand fixes committed; Results list residual risks; status updated.

## Results

- Reviewed both regions across `common`, `sidebar`, `settings`, `task`,
  `tasks`, `auth`, `github`, `workspaces`, and `system` using targeted term
  scans plus representative copy inspection.
- Fixed glossary expansion (`工作流程程`), mainland-oriented software terms,
  and contextual OpenCC misses. Examples include `設定檔`, `新增`, `訊息`,
  `佇列`, `控制項`, `執行`, `程式碼`, plus Taiwan-specific `清單`, `檢視`,
  `螢幕`, and `意見回饋`.
- Added a reviewed per-key correction for the data-directory help text where a
  global token rule would be unsafe.
- Critical malformed/mainland marker scan is empty. The nine reviewed
  namespaces retain 891 TW/HK value differences.
- Residual risk: production E2E proves switching and persistence, but a final
  native-speaker visual pass on dense layouts remains a release acceptance
  step rather than an automated guarantee.
- Follow-up UI terminology review replaced opaque component calques such as
  `手风琴树`, `持久树`, `菜单形态`, `面包屑`, and `状态抽屉` with labels that
  describe their visible behavior.
