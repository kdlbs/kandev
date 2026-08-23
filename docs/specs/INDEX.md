# Specification catalog

This catalog is the entry point for the system-oriented specification layout. Each system README defines ownership, exclusions, and links to its canonical requirements and system designs.

## Systems

| System | Index | Migration | Canonical documents |
| --- | --- | --- | --- |
| Agents | [README](agents/README.md) | complete | 25 requirements, 15 designs |
| Auth | [README](auth/README.md) | complete | 7 requirements, 2 designs |
| CLI | [README](cli/README.md) | complete | 2 requirements, 0 designs |
| Costs | [README](costs/README.md) | complete | 2 requirements, 0 designs |
| Desktop | [README](desktop/README.md) | complete | 1 requirements, 1 designs |
| Executors | [README](executors/README.md) | complete | 3 requirements, 6 designs |
| Integrations | [README](integrations/README.md) | complete | 18 requirements, 21 designs |
| Office | [README](office/README.md) | complete | 19 requirements, 38 designs |
| Platform | [README](platform/README.md) | complete | 30 requirements, 10 designs |
| Plugins | [README](plugins/README.md) | complete | 7 requirements, 9 designs |
| Release | [README](release/README.md) | complete | 5 requirements, 0 designs |
| System page | [README](system-page/README.md) | complete | 3 requirements, 5 designs |
| Tasks | [README](tasks/README.md) | complete | 68 requirements, 12 designs |
| UI | [README](ui/README.md) | complete | 95 requirements, 11 designs |
| Workspaces | [README](workspaces/README.md) | complete | 10 requirements, 2 designs |

## Migration record

The legacy specification sources were migrated from the unstructured root and category directories into the system-oriented layout. Task and workflow sources from PR #2957 remain under `tasks/`; three task-owned sources found outside that tree were folded into the same system during this migration.

----

The former legacy size exceptions were removed. All canonical requirement and system-design documents now pass the repository specification linter.

## Authoring rule

New behavior belongs in a system README's `requirements/` directory, with technical boundaries in `system-design/`. Do not add new `spec.md` files or parallel category-owned sources.
