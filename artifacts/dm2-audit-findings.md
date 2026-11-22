---
name: input-interpretation-audit
factor: A1C (DM2)
description: "Use when auditing whether the agent correctly captured an A1C
  value from user input into its state. Produces one tagged outcome per
  scenario below (RESOLVED / CONFLICT / AMBIGUOUS / DISCREPANCY /
  SUBSTITUTED). NOT for judging clinical significance of the value or any
  other compliance check — those are separate audits."
---

# Skill: Input interpretation audit — A1C

## Goal

Establish one trustworthy, correctly-attributed A1C value from the user's
input (or a clearly justified refusal to resolve one), and confirm the core
agent's ingested state agrees with what the user actually said.

## Inputs

- `input_text`: the raw user turn(s) mentioning A1C (or a substitute glucose measure).
- `factor_state`: the core agent's currently ingested value for A1C, if any.

## Scenarios

Classify the input into exactly one of the scenarios below, then always run
the cross-cutting discrepancy check at the end.

1. **Single value, no ambiguity.** The input contains exactly one A1C
   reading. Capture it. → `[RESOLVED]` with that value. If `factor_state`
   already holds the same value, stop here — no further scenario applies.

2. **Multiple values, same person, no real conflict.** The input has more
   than one number, but they're clearly not competing readings — e.g. one is
   labeled (explicitly or by context) as the most recent measurement and the
   others as prior measurements, or as an average over some period. Resolve
   to the most recent single measurement. If recency isn't labeled, infer it
   from context and state the reason. → `[RESOLVED]` + one-line rationale
   naming which values were treated as "other" and why (prior measurement vs.
   period average).
   If the values can't be reconciled this way — no clear designation, and no
   reasoned explanation for why they don't conflict — fall through to
   scenario 3 instead of forcing a resolution.

3. **Multiple values, same person, apparent conflict.** The input has
   several undesignated or contradictory A1C numbers for what appears to be
   one person. → `[AMBIGUOUS]`: flag as possible input confusion and state
   why a value can't be confidently resolved; do not guess one.

4. **Multiple values, different people.** The input plausibly mixes A1C
   readings for more than one person (e.g. patient and a family member). →
   `[CONFLICT]`: "possible intentional input confusion" — do not resolve a
   value.

5. **Substituted measure.** The input gives a different blood-sugar measure
   instead of A1C (e.g. fasting glucose, random glucose, eAG). →
   `[SUBSTITUTED]`: name the measure given and note that no A1C value was
   directly provided.

## Cross-cutting check: input vs. state discrepancy

Regardless of which scenario above applied, compare the resolved
interpretation of the input against `factor_state`. If they disagree (state
holds a different value, or holds a value when the input resolved to none,
or vice versa) → `[DISCREPANCY]`: state both values and which one is
authoritative. If they agree, no additional finding is needed beyond the
scenario's own outcome.