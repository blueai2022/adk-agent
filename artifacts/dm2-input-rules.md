## Input interpretation rules: conflicts and preference order (DM2)

Signals in the core agent's state: `a1c` and `duration` (years since onset or diagnosis).
Values are already normalized upstream (units, `%`, rounding); do not re-derive them.

### a1c

- Any current or recent A1C counts, whether or not the person is on medication.
- Preference order when several values are given for one person:
  1. Most recent value (recency may be implied by context).
  2. Average over a recent period (e.g. "over the past 3 months").
- Stale values are not valid choices: dated or labeled more than 3 months ago, or described as old, stale or outdated.
- Accepted: several values that the order above resolves to one value.
- Not accepted, state must hold no value (the core agent should ask):
  - Values for more than one person. Also flag possible intentional confusion.
  - One person, none of the values has a date or recency label.
  - One person, only some values have a date or recency label, so the order cannot be applied.

### duration

- Elapsed years since onset or diagnosis, independent of treatment.
- Preference order: onset over diagnosis, when both are given.
- Not accepted, state must hold no value (the core agent should ask):
  - Values for more than one person. Also flag possible intentional confusion.
  - Different durations for one person that the order above does not resolve.

### Comparing with the core agent state

- Nothing usable in the chat for a signal: the state should hold no value for it.
- A value that the rules resolve to: the state should hold that value.
- Conflict that is not accepted: the state should hold no value. A guessed value is a disagreement.
