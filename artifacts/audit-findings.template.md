<!--
Authoring template — not consumed by the runtime audit LLM.

Use this to produce a new `<factor>-audit-findings.md` (see
dm2-audit-findings.md for a filled-out example). Fill in every `{{TOKEN}}`,
then delete this comment block. The result handed to the audit LLM must stay
as flat and concrete as dm2-audit-findings.md — no extra layers, no
meta-talk about "this is a template." The runtime LLM only ever sees the
finished, factor-specific form.

Tokens:
  {{FACTOR_LABEL}}     human name, e.g. "A1C (DM2)" or "Blood pressure (HTN)"
  {{FACTOR_ICD10}}     e.g. "E11.9"
  {{MEASURE_NAME}}     the value being audited, e.g. "A1C", "systolic/diastolic BP"
  {{STATE_KEY}}        key in factor_state this maps to, e.g. "A1C"

Scenarios 4 and 5 are optional — most factors won't need both:
  - Keep scenario 4 (different people) only if this measure is commonly
    reported for more than one household member in the same conversation
    (vitals like A1C, BP, BMI). Drop it for things like age or tobacco_use.
  - Keep scenario 5 (substituted measure) only if there's a real alternate
    measure users might give instead (e.g. fasting glucose instead of A1C).
    Drop it if no such substitute exists.
  If you drop a scenario, remove it entirely and renumber the rest — don't
  leave a gap or an "N/A" placeholder in the finished form.
-->

---
name: input-interpretation-audit
factor: {{FACTOR_LABEL}}
description: "Use when auditing whether the agent correctly captured {{MEASURE_NAME}}
  from user input into its state. Produces one tagged outcome per scenario
  below (RESOLVED / CONFLICT / AMBIGUOUS / DISCREPANCY, plus SUBSTITUTED if
  scenario 5 applies). NOT for judging clinical significance of the value or
  any other compliance check — those are separate audits."
---

# Skill: Input interpretation audit — {{MEASURE_NAME}}

## Goal

Establish one trustworthy, correctly-attributed {{MEASURE_NAME}} value from
the user's input (or a clearly justified refusal to resolve one), and
confirm the core agent's ingested state agrees with what the user actually
said.

## Inputs

- `input_text`: the raw user turn(s) mentioning {{MEASURE_NAME}}.
- `factor_state`: the core agent's currently ingested value for
  `{{STATE_KEY}}`, if any.

## Scenarios

Classify the input into exactly one of the scenarios below, then always run
the cross-cutting discrepancy check at the end.

1. **Single value, no ambiguity.** The input contains exactly one
   {{MEASURE_NAME}} reading. Capture it. → `[RESOLVED]` with that value. If
   `factor_state` already holds the same value, stop here — no further
   scenario applies.

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
   several undesignated or contradictory {{MEASURE_NAME}} numbers for what
   appears to be one person. → `[AMBIGUOUS]`: flag as possible input
   confusion and state why a value can't be confidently resolved; do not
   guess one.

4. *(Optional — keep only if this measure is plausibly reported for more
   than one person in one conversation.)* **Multiple values, different
   people.** The input plausibly mixes {{MEASURE_NAME}} readings for more
   than one person (e.g. patient and a family member). → `[CONFLICT]`:
   "possible intentional input confusion" — do not resolve a value.

5. *(Optional — keep only if a real alternate measure exists for this
   factor.)* **Substituted measure.** The input gives a different measure
   instead of {{MEASURE_NAME}} (e.g. [alternate measure(s)]). →
   `[SUBSTITUTED]`: name the measure given and note that no {{MEASURE_NAME}}
   value was directly provided.

## Cross-cutting check: input vs. state discrepancy

Regardless of which scenario above applied, compare the resolved
interpretation of the input against `factor_state`. If they disagree (state
holds a different value, or holds a value when the input resolved to none,
or vice versa) → `[DISCREPANCY]`: state both values and which one is
authoritative. If they agree, no additional finding is needed beyond the
scenario's own outcome.
