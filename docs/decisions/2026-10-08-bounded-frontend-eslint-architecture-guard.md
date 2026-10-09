# ADR-2026-10-08-bounded-frontend-eslint-architecture-guard: Bounded frontend ESLint architecture guard

**Status:** accepted
**Date:** 2026-10-08
**Area:** frontend

## Context

Architecture lint currently uses a dependency-free Python engine for its
registered repository rules, focused Python tests, and exact shrink-only
baselines. LINT-02 protects four accepted frontend ownership contracts whose
authoritative owners are TypeScript declarations, Zustand slice objects, and
hydration writes. The web app already uses ESLint with the installed
typescript-eslint parser and has local custom-rule and RuleTester patterns.

The [migrated System Query owner design](../specs/architecture-lint/system-design/migrated-system-query-owner.md)
defines the exact guarded syntax and boundaries. The four accepted resource
designs remain authoritative for ownership behavior.

## Decision

For this bounded LINT-02 guard, use one scoped custom ESLint AST rule and the
existing web RuleTester and lint configuration. Register it only on the four
agreed System state and hydration owner files, and run it through existing web
lint and frontend CI. Keep the Python architecture-lint registry, scanner
modules, tests, baselines, shrink-only checks, and entry points unchanged.

The accepted guard design defines the exact source boundary and supported forms.

## Consequences

The guard uses an installed TypeScript-aware parser, adds no dependency or
baseline, and gets diagnostics from ESLint at the source location. Its scope is
limited to statically recognizable shapes in four named owner files. It does
not prove aliases, dynamic keys, arbitrary data flow, or mirrors outside those
paths. Frontend CI and web tests verify rule execution and configuration;
make lint-architecture continues to run only the Python architecture engine.

## Alternatives Considered

- **Add a Python source scanner:** Rejected for this rule because the engine's
  current text scanning would need a separate TypeScript syntax strategy to
  distinguish owner structure from comments, strings, unrelated objects, and
  static computed keys.
- **Add a TypeScript parser or lexer to the Python engine:** Rejected because
  it adds a new parsing and maintenance surface when the web app already
  installs a TypeScript-aware ESLint parser.
- **Use only a standalone structural test:** Rejected because ordinary web
  lint and changed-owner pre-commit lint would not enforce the boundary on
  implementation changes.
