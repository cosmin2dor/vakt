# Vakt

A file-reactive household task orchestrator with no database. A directory of Markdown files (the *Vault*) is the single source of truth. A Go daemon watches it, schedules tasks from inline `@directives`, dispatches push notifications, and writes state back into the same plain text. The frontend is a React PWA.

Go backend + TypeScript/React PWA, one container, two volumes (`/vault` for user content, `/config` for system state).

---

## The documents

Read the one that governs your question. Don't re-derive an answer that's already written down.

| File | Authoritative for | Read it when |
|---|---|---|
| `PRD.md` | Product intent, directive syntax, functional requirements | You need to know *what* a feature is or *why* it exists |
| `SDD.md` | Architecture, package layout, and every resolved specification gap | You need to know *how* the system is built or how an edge case behaves |
| `UX.md` | Design tokens, typography, motion, component mapping | You are writing anything the user sees |
| `issues/milestone*.md` | The work, in order | You are picking up a task |

**Precedence when they disagree:** `SDD.md` over `PRD.md` (the SDD resolves things the PRD left open), and the issue's own description over both (it carries scope boundaries the documents don't).

### Reading SDD.md

Two sections matter most:

- **§3 Resolved specification gaps (G1–G15)** — rulings on things the PRD left ambiguous: task identity, duplicate `@id`, the suppression precedence ladder, missed triggers, timezone behaviour. If your task touches scheduling or state, read these first. They are decisions, not suggestions.
- **§2 Architecture** — the package layout and the one principle everything else rests on.

### The principle that gets broken by accident

**The backend is the only parser.** The browser never interprets a directive. No cron library in the frontend, no computing when a task fires next, no validating a directive client-side. Every semantic question goes to `POST /api/v1/parse`; the editor's autocomplete reads `GET /api/v1/directives` at runtime.

If you find yourself reaching for a date or cron library in `web/`, stop — the answer belongs on the server.

### Reading UX.md

Every colour resolves through a CSS token. A hex literal in a component is a bug. The app ships **one theme, dark** — there is no light palette and no toggle.

---

## The issue board

`issues/milestone1.md` (and later milestones) hold the work. Tasks are listed in **topological order** — every blocker appears above the task it blocks.

Format:

```markdown
- [ ] task-slug \
Description: What to build, and where its scope ends. \
Attached PR: — \
Blocked by: @other-slug, @another-slug
```

A task with no `Blocked by` line has no blockers.

### Working a task

1. Take the **first unchecked task whose blockers are all checked**. Don't skip ahead — the order is a real dependency graph, not a preference.
2. Read the description in full. It contains the scope boundary as well as the work.
3. Implement it. Consult `SDD.md` for architecture, `UX.md` for anything visual.
4. Open a PR.
5. Replace the `—` on the `Attached PR:` line with the link: `Attached PR: [PR-14](https://github.com/.../pull/14)`
6. Tick the checkbox: `- [x] task-slug`

If a task turns out to need work that isn't in its description, say so rather than quietly widening it.

### One concern per commit, one commit per PR

Optimize for the reviewer. A diff should be readable in one sitting and hold exactly one idea.

- **One commit per PR.** Squash as you go; don't ship a PR with a history of fixups.
- **One concern per commit.** Never combine unrelated changes — a refactor and a feature, two independent endpoints, a rename and a behaviour change — in the same commit or the same PR.
- **A task with several concerns becomes several PRs, stacked.** Each PR branches off the previous one rather than off `main`, and merges in order. A task is done when its whole stack has landed.
- **Conventional commit prefix.** Every commit subject starts with its type: `chore:`, `feat:`, `fix:`, `docs:`, `test:`, `refactor:` — e.g. `chore: Add Service X`. The prefix names the single concern above; if a commit needs two prefixes, it's two commits.

**Always use `gh stack` to create, update, and land stacks.** It is a `gh` extension — install it if it's missing. Never hand-roll a stack with raw `git push` and `gh pr create`: the base branches and cross-links between PRs have to stay consistent as the stack is rebased, and doing that by hand is where stacks break. Check `gh stack --help` for the current subcommands rather than assuming them.

Record the whole stack on the board, in merge order:

```markdown
Attached PR: [PR-14](link), [PR-15](link), [PR-16](link)
```

Splitting a task into a stack is expected and good. Widening a task's scope is not — the two look similar in a diff, so name which one you're doing in the PR description.

### Scope boundaries are load-bearing

Task descriptions state where work stops, and those limits exist to keep milestones honest. Examples from M1: the trigger endpoint **does not write back** (the surgical patcher is M2), the push module sends a **fixed payload** (templating is M3), the directive lexer handles **spans only** (typed values and validation are M2).

Each of these is a natural instinct to follow and each would pull a later milestone's work into the current one. If a task feels incomplete, that is usually deliberate — check the milestone summary in `SDD.md` §4 before adding anything.

---

## Conventions

- Types are **generated**, never hand-written. `schema/openapi.yaml` and `schema/directives.yaml` are the source; `make generate` produces Go and TypeScript. CI fails if committed output drifts.
- Go packages follow `SDD.md` §2.6. `internal/engine/` is the core and knows nothing about HTTP or any notification provider.
- Never write system state into `/vault`. VAPID keys and push subscriptions live on `/config`.
- Never re-serialize a Markdown file from its parsed model. Writes are byte-range patches to a single directive span.
