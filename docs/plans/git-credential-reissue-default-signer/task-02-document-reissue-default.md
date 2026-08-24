---
id: "02-document-reissue-default"
title: "Document Git credential reissue defaults"
status: done
wave: 2
depends_on: ["01-default-reissue-signer"]
plan: "plan.md"
spec: "../../specs/git-credential-lease-reissue/spec.md"
---

# Task 02: Document Git credential reissue defaults

## Acceptance

1. Public configuration and executor documentation states that an omitted
   stable key enables same-process recovery through an automatic process-local
   signer.
2. Docker and Kubernetes operator guidance still requires the stable key for
   cross-backend-restart recovery and warns that rotating it invalidates
   outstanding capabilities.
3. Documentation never implies that connection refresh widens repository scope,
   changes task credential policy, or repairs sessions launched before the fix.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
rg -n 'reissueSigningKey|REISSUE_SIGNING_KEY|process-local|backend restart' docs/public/configuration.md docs/public/executors.md docs/public/integrations.md docs/docker.md docs/k8s.md
```

## Files likely touched

- `docs/public/configuration.md`
- `docs/public/executors.md`
- `docs/public/integrations.md`
- `docs/docker.md`
- `docs/k8s.md`

## Dependencies

Task 01.

## Parallelism

Sequential.

## Inputs

- Spec sections: `Persistence guarantees`, `Scenarios`, and `Out of scope`.
- Plan sections: `Public documentation` and `Risks`.
- `docs/public/configuration.md` is reference content;
  `docs/public/executors.md`, `docs/docker.md`, and `docs/k8s.md` are operator
  how-to guidance.

## Output contract

Report changed documentation, exact validation results, any broken-link or
navigation findings, and synchronized task/plan status.

## Results

- `node --test scripts/validate-public-docs.test.mjs` passed 61/61 tests.
- `node scripts/validate-public-docs.mjs` validated 41 published documentation pages.
- The required terminology scan found the automatic process-local behavior and stable cross-restart key guidance in configuration, executor, integration, Docker, and Kubernetes documentation.
- Public docs updated: `docs/public/configuration.md`, `docs/public/executors.md`, and `docs/public/integrations.md`.
- Operator docs updated: `docs/docker.md` and `docs/k8s.md`.
- External side effects: None.
