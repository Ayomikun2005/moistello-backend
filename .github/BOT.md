# Moistello Bot Setup & Usage

The bot runs as **GitHub Actions workflows** inside this repo and is named **moistello**. It handles:

| What | Workflow | Trigger |
|------|----------|---------|
| Lint + test + build gate | `ci.yml` | every PR / push |
| **Auto-merge** 30 min after CI is green | `moistello: Auto-Merge` | CI success (30m delay), `/merge` comment |
| **Auto-fix** conflicts + failing CI | `moistello: Auto-Fix` | PR conflict, CI failure (PR *and* `main`) |

The bot **never assigns issues or reviewers** — that stays manual (you decide who implements what).

---

## Free-tier note (your concern)

- Both `moistello-*` repos are **public** → GitHub Actions minutes are **unlimited** (no charge).
- **Auto-merge** is a native GitHub feature available on the free plan for public repos.
- All workflows are **webhook-driven** (no polling), so they don't hit REST API rate limits.
- The only cost, if you want it, is the **LLM API key** used by the AI auto-fix step
  (you can leave it unset and the bot will still rebase + auto-merge, just not AI-fix).

## Secrets to configure (Settings → Secrets and variables → Actions)

| Secret | Required? | Purpose |
|--------|-----------|---------|
| `OPENCODE_API_KEY` | No* | Enables AI auto-fix of failing CI. Skip → bot rebases/merges only. |
| `OPENCODE_MODEL` | No | Override model (default `opencode-go/deepseek-v4-flash`, the cheapest capable coding model). |
| `BOT_TOKEN` | No | A PAT with `repo` scope; enables pushing to protected branches. Falls back to the built-in `GITHUB_TOKEN`. |

\* Only set `OPENCODE_API_KEY` if you want the bot to actually edit code to fix CI. Without it, CI failures are flagged for a human (you) to fix — which guarantees a human implements the work.

## Recommended branch protection (to make auto-merge safe)

1. Repo → Settings → Branches → Add rule → default branch.
2. Require: `status checks to pass before merging` → select **CI** checks.
3. Optionally require 1 approving review if you want a human gate before the bot merges
   (set `require_human_review: true` in `.github/bot-config.yml` to match).

With auto-merge, the **moistello** bot merges a linked PR **30 minutes after CI passes** (set `delay_minutes` in `.github/bot-config.yml`). If branch protection requires a human review, the bot waits for that review before the 30-minute countdown completes.

## Auto-fix behavior (opencode codes ONLY on merge conflicts)

The **moistello** bot never auto-writes code on ordinary CI failures — those are left for you.
It only runs opencode when a PR has a **merge conflict**:

1. **Merge conflicts**: on a PR with conflicts, it first tries a clean rebase. If a rebase
   isn't possible, it hands the conflicted repo (full codebase, all history) to an opencode
   agent that resolves the conflict correctly, completes the rebase, and pushes the fix.
2. **CI failures that are NOT conflicts** → left for a human. This is how you stay in control
   of what gets written.

The workflow checks out the **entire repository with full history** (not just the PR head)
so the agent has complete codebase context, and caches Go modules / npm + the opencode CLI
between runs to avoid re-downloads and cut cost.

### Off-peak waiting (cost saver)

DeepSeek V4 switches to **peak/off-peak pricing** (peak = 2.4–4.7x the off-peak rate):
- **Peak hours:** 01:00–04:00 and 06:00–10:00 UTC
- **Off-peak hours:** the other 17 hours/day at the cheapest rate

When a merge conflict needs AI resolution during a **peak hour**, the moistello bot
**waits until the next off-peak hour starts**, then runs opencode at the cheaper rate.
Toggle this with `wait_for_off_peak` in `.github/bot-config.yml`.

## Manual merge command

Anyone with write access can comment `/merge` on a PR to enable auto-merge.

## Ignore rules

PRs whose title contains `[wip]`, `[do not merge]`, or `draft` are never auto-merged.