---
name: commit
description: >
  Smart commit with Conventional Commits: groups logical changes,
  infers type + scope from file paths, and generates messages in
  `<type>(<scope>): <description>` format.
argument-hint: "Describe the changes in one line / or /commit for interactive"
---

# Commit

**Leading word:** _atomic_ — one logical change per commit, one clear message per change.

Group related file changes into atomic commits with conventional messages. The `/commit` command (or this skill) analyses `git status`, detects change patterns, infers the correct type + scope, and writes a structured message.

## Conventional Commit Format

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

### Types

| Type       | When to Use                                          |
|------------|------------------------------------------------------|
| `feat`     | A new feature for the user                           |
| `fix`      | A bug fix                                            |
| `docs`     | Documentation only changes                           |
| `style`    | Formatting, whitespace, missing semicolons (no logic) |
| `refactor` | Code change that neither fixes nor adds functionality |
| `perf`     | Performance improvement                              |
| `test`     | Adding/improving tests                               |
| `build`    | Build system or external dependency changes          |
| `ci`       | CI configuration and scripts                         |
| `chore`    | Other changes (config, tooling, meta)                |
| `revert`   | Reverts a previous commit                            |

Source: [conventionalcommits.org](https://www.conventionalcommits.org/)

### Scope Examples

Infer scope from the directory/module that got the most changes:

| Pattern               | Scope         |
|-----------------------|---------------|
| `.pi/skills/*`        | `skill`       |
| `.pi/extensions/*`    | `extensions`  |
| specifically `lemonharness/` | `memory`, `workspace`, `search`, `quality-gate`, `ui`, `delegate` |
| `src/lib/`            | `lib`         |
| `tests/`              | `tests`       |
| `docs/`               | `docs`        |
| `.github/`            | `ci`          |

## Rules

1. **One logical change per commit** — If unrelated files are changed, split into multiple commits.
2. **Infer type from content** — `.md` files → `docs`, test files → `test`, config → `chore`, new features → `feat`.
3. **Scope from directory** — the deepest common directory of changed files is the best scope hint.
4. **Description is imperative** — "add", "fix", "remove", not "added", "fixed", "removed".
5. **Description is lowercase** — no capital first letter. No trailing period.
6. **Max 72 chars** for the subject line. Wrap body at 72 chars.
7. **Body explains why, not what** — the diff shows what changed. The body explains the motivation.
8. **Footer for breaking changes** — `BREAKING CHANGE:` footer when the API contract changes.

## Auto-Detect Keywords

commit, conventional commit, smart commit, git commit, commit message, staged, changes, diff

## Pseudocode Contract

```
SKILL commit
INPUTS:
  git_status: string // output of `git status --porcelain`
  user_summary: string (optional) // user-provided description hint
OUTPUTS:
  commit_message: string // generated conventional commit message
  grouped_changes: array // files grouped by logical change
PRECONDITIONS:
  - git is available
  - there are staged or unstaged changes
POSTCONDITIONS:
  - commit_message matches <type>(<scope>): <description>
  - scope is inferred from file paths
  - description is imperative, lowercase, ≤72 chars
  - if multiple logical groups exist, user is prompted to choose
ERROR_HANDLING:
  - no changes: inform user, do nothing
  - ambiguous scope: prefer root module name
  - unknown type: default to "chore"
```
