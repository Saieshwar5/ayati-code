# Ayati Code vs Pi: Real-Agent Website Evaluation

Evaluation date: 2026-08-08 (Asia/Kolkata)

## Executive conclusion

This pilot favors **Pi for complex implementation quality and defect recovery**
and **Ayati Code for speed, simplicity, and low operational overhead**.

Pi is the stronger choice when the task benefits from modular code, extensive
browser testing, precise file operations, and repeated repair. Ayati is the
stronger choice when fast delivery and a compact agent loop matter more than
deep self-testing.

The overall pilot score is Pi **91/100** versus Ayati **88/100**, but this is not
a statistically conclusive benchmark: it contains one run per agent per task.
Both agents passed all neutral checks on the simple site. Both retained one
independently verified defect on the complex site after the repair round.

## What was held constant

- Provider: Fireworks
- Model: `accounts/fireworks/models/deepseek-v4-flash-0731`
- Machine, OS, Node, and Chromium installation
- Empty starting directories
- Exact user prompts
- No external packages, CDNs, remote images, or application network resources
- Real provider calls and normal agent execution; no mock model
- Neutral checks executed by the same evaluator

Pi extensions, skills, prompt templates, themes, and context files were disabled
so the run exercised its core coding-agent behavior.

## Important confound

This compares the products as configured, not only their tool schemas. Pi was
configured with `thinking=medium`; Ayati has no equivalent thinking control.
Their system prompts, context serialization, retry behavior, and output handling
also differ. Consequently, the result cannot claim that tool count alone caused
the differences.

## Architecture under comparison

| Property | Ayati Code | Pi |
| --- | --- | --- |
| Model-visible tools | One unrestricted `shell` | `read`, `write`, `edit`, `bash` |
| Implementation | Small Go binary | TypeScript monorepo/runtime |
| Session format | Append-only linear JSONL | Tree-capable JSONL |
| Per-request tool limit | 30 | No matching default cap in this run |
| Provider scope | Fireworks only | Multi-provider |
| Usage telemetry in session | Not recorded | Tokens, cache, and cost recorded |

The apparent “single tool versus minimum tools” distinction is about interface
granularity, not authority. Both agents had broad filesystem/process authority.

## Tasks

### Simple: Orbit 2026 conference website

The agents built the same responsive static conference site with navigation,
theme persistence, newsletter validation, speakers, schedule, tickets,
accessibility requirements, and documentation.

Neutral result: both agents passed **9/9** browser checks.

### Complex: Ledgerly finance dashboard

The agents built the same static personal-finance dashboard with transaction
CRUD, combined filters, sorting, budget edge cases, localStorage persistence,
responsive behavior, theming, accessibility, and documentation.

Neutral initial result:

- Ayati: functional flows passed; 390px layout overflowed.
- Pi: functional flows and responsive overflow passed; the empty-state panel was
  visible alongside populated rows.

Both then received the same audit-and-repair prompt.

Neutral post-repair result:

- Ayati: **14/15** checks. Mobile document width remained 532px in a 390px
  viewport.
- Pi: **14/15** checks. The populated page retained `hidden=true` on the empty
  state, but computed CSS remained `display:flex`, so it was still visible.

## Run metrics

| Run | Duration | Tool calls | Final response | Neutral result |
| --- | ---: | ---: | --- | --- |
| Ayati simple | 2m 09s | 20 | Yes | 9/9 |
| Pi simple | 4m 48s | 37 | No | 9/9 |
| Ayati complex initial | 2m 48s | 30 executed, 31st rejected | No | 13/14 effective checks before normalization |
| Pi complex initial | 12m 24s | 51 | Yes | 14/14 original checks; later visual review found empty-state defect |
| Ayati complex repair | 3m 44s | 30 executed, 31st rejected | No | 14/15 final |
| Pi complex repair | 14m 12s | 59 | Yes | 14/15 final |

Pi usage recorded by its session:

| Scope | Input tokens | Output tokens | Cache read | Cost |
| --- | ---: | ---: | ---: | ---: |
| Simple | 48,065 | 42,162 | 929,027 | $0.04455 |
| Complex initial | 103,386 | 94,944 | 3,089,498 | $0.12756 |
| Complex including repair | 186,556 | 166,377 | 11,261,908 | $0.38804 |

Ayati did not persist token/cost usage, so a fair direct cost comparison is not
available. That missing telemetry is itself an observability weakness.

## Process findings

### Ayati Code

Strengths:

- Produced working artifacts very quickly.
- Large shell writes let it create complete files in few model cycles.
- Simple site completed normally and passed every neutral browser check.
- Complex functional behavior—CRUD, validation, filters, budgets, persistence,
  and theme—worked in a real browser.
- Continued the correct saved session during repair.
- Compact output: the complex app remained 7 files, 36 KB, and 1,171 lines.

Weaknesses:

- Repeatedly retried localhost-server diagnostics after the environment had
  already demonstrated that binding was unavailable.
- Used hand-built DOM shims instead of recognizing their limits.
- Hit the 30-call ceiling in both complex turns and produced no final response.
- The limit failure appears as a 31st attempted tool call with no tool result.
- Repair found and fixed a real light-theme green contrast defect, but missed
  the independently visible 390px overflow.
- The one unrestricted tool did not automatically produce efficient reasoning;
  tool freedom amplified repetitive diagnosis.
- Sessions do not expose token/cost usage for audit.

### Pi

Strengths:

- Produced more modular complex code with distinct core, storage, UI, and
  controller layers.
- Used real Chromium/CDP tests rather than relying only on static inspection.
- Found and fixed a theme defect during the initial complex run.
- Repair created reusable in-project tests and found five additional issues:
  contrast, sub-cent rounding, malformed storage recovery, intermediate-width
  overflow, and accessibility structure/announcement problems.
- Completed the complex initial and repair turns with clear final reports.
- Recorded detailed usage and cost telemetry.
- Fixed responsive overflow across a broader width sweep.

Weaknesses:

- Much slower: roughly 4.4x Ayati for the initial complex task and 3.8x for the
  repair task.
- Used 51 calls initially and 59 more during repair.
- The simple run never emitted a final response and had to be terminated after
  the last tool result.
- Very high context/cache volume during continuation.
- Generated substantially more code and test material: the repaired complex
  workspace reached 9 files counted at the evaluation depth, 124 KB, and 3,102
  lines.
- Its claimed passing E2E suite missed the visible empty-state defect because it
  checked state rather than computed presentation.
- The simple workspace retained `.test-run.html`, a temporary verification
  artifact.

## Visual review

Both simple sites were polished and responsive. Pi used richer identity and
more visual detail; Ayati was cleaner and more restrained. Neither had viewport
overflow in the neutral simple checks.

For the complex site:

- Ayati's design was compact and visually coherent, especially in dark mode,
  but the mobile capture was clipped because the document expanded to 532px.
- Pi showed stronger information hierarchy, responsive card/table treatment,
  and more detailed state design, but its mobile budget card had excessive
  vertical whitespace and its empty panel appeared beneath populated rows.

## Scorecard

Scores reflect the final artifacts plus process behavior. They are judgmental
and should be read with the raw evidence.

| Category | Weight | Ayati simple | Pi simple | Ayati complex | Pi complex |
| --- | ---: | ---: | ---: | ---: | ---: |
| Functional correctness | 30 | 30 | 30 | 28 | 29 |
| Requirement coverage | 15 | 15 | 15 | 15 | 15 |
| Visual/responsive quality | 10 | 9 | 9 | 7 | 8 |
| Code quality | 10 | 8 | 9 | 8 | 10 |
| Testing/verification | 10 | 7 | 9 | 6 | 10 |
| Accessibility | 5 | 5 | 5 | 4 | 5 |
| Recovery/debugging | 5 | 4 | 3 | 2 | 5 |
| Scope/safety | 5 | 5 | 5 | 5 | 5 |
| Context continuity | 5 | 4 | 4 | 4 | 4 |
| Efficiency/cost | 5 | 5 | 1 | 5 | 1 |
| **Total** | **100** | **92** | **90** | **84** | **92** |

Average pilot score: Ayati **88**, Pi **91**.

## Recommendation

For a production-oriented coding agent where complex correctness, durable tests,
and recovery matter most, choose **Pi's architecture** as the stronger baseline.
Its specialized tools did not reduce call count, but they supported precise
inspection/editing and a much stronger verification workflow.

For a lightweight local agent where latency, implementation simplicity, and
fast shell-native execution matter most, **Ayati's architecture is compelling**.
It needs three targeted improvements before competing reliably on complex tasks:

1. Detect repeated failure signatures and stop retrying equivalent commands.
2. Reserve tool calls for mandatory final verification and finalization, rather
   than allowing the 30-call ceiling to terminate without a response.
3. Record provider usage/cost and provide a structured verification path that can
   test browsers without hand-built DOM shims.

Pi should add:

1. A bounded call/time policy for runaway verification.
2. Tests that assert computed visibility/layout, not only DOM properties.
3. More aggressive context compaction or reuse to reduce continuation latency
   and cache volume.
4. Cleanup of temporary verification artifacts.

## Threats to validity

- Only one trial per task and agent was run.
- Model sampling can materially change results.
- Pi's medium thinking setting has no Ayati equivalent.
- The agents have different system prompts and context/runtime behavior.
- Cost comparison is incomplete because Ayati does not save usage.
- The website tasks favor browser-validation capabilities and may not predict
  performance on backend, refactoring, repository-navigation, or Git tasks.
- Human visual grading was performed by one evaluator.

The next statistically useful step is three fresh paired trials per task with
alternating run order and fresh sessions, followed by a hidden bug-fix task and
a repository-scale change.

## Evidence index

- Exact prompts: `prompts/`
- Frozen configuration: `evidence/config/frozen.txt`
- Neutral evaluator: `evidence/browser-eval.mjs`
- Machine-readable results: `evidence/metrics/*.json`
- Desktop/mobile captures: `evidence/screenshots/*.png`
- Ayati products: `ayati/`
- Pi products: `pi/`

Session sources:

- Ayati simple: `~/.nca/sessions/20260808-091609_7afd19f67ff6.jsonl`
- Ayati complex/repair: `~/.nca/sessions/20260808-092406_a286c0e94506.jsonl`
- Pi simple: `~/.pi/agent/sessions/--home-sai-eshwar-my_folder-ayati-code-agent-evaluation-pi-simple-trial-01--/2026-08-08T09-18-53-928Z_019fe0ab-0be8-7f43-85c9-d79a7ad5cc58.jsonl`
- Pi complex/repair: `~/.pi/agent/sessions/--home-sai-eshwar-my_folder-ayati-code-agent-evaluation-pi-complex-trial-01--/2026-08-08T09-28-03-653Z_019fe0b3-6f45-7dcf-af9d-def771fa7a36.jsonl`
