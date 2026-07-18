---
name: commit
description: Analyses diffs, groups logical changes, and commits each group with a conventional commit message.
---

# Commit

When the user asks to commit, automatically group related changes into separate conventional commits.

## Process

1. **Analyse** — Run `git diff --stat` and `git diff` (or `git diff --cached` if things are staged) to understand all current changes.

2. **Group logically** — Partition changes into logical groups. For example:
   - A new feature spanning multiple files → one group
   - A bug fix in a separate file → another group  
   - Schema/migration changes → one group
   - Refactoring that touches several files → one group
   - Configuration or tooling changes → one group
   - Documentation updates → one group

3. **For each group**, determine:
   - **type** — `feat`, `fix`, `chore`, `docs`, `refactor`, `style`, `test`, `perf`
   - **scope** (optional) — e.g. `ui`, `handler`, `db`, `api`, `feeds`
   - **description** — short imperative summary of what this group does

4. **Commit each group** — Run `git add <files-in-group>` then `git commit -m "type(scope): description"`. If not all changes are staged yet, stage only the files for the current group.

5. Show the user a summary of all commits made.

## Principles

- Keep commits focused: one logical change per commit.
- A group may be a single file or span many files — what matters is that they form one coherent change.
- Use scopes that match the project's architecture (e.g. `ui`, `handler`, `db`, `feeds`, `auth`).
- If the user provides explicit instructions ("commit as feat(ui): ..."), follow their lead and group accordingly.
