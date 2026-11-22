## 1. Input interpretation audit rules

### A1C Value

- Input Validity:
The person's current/recent A1C value is what is expected, irrespective of whether the patient is currently on medication or not, i.e. a good A1C value with or without medication is considered a valid input. See stale values section for handling ``Explicitly non-recent A1C values``.

- Alternative measurement units
    - eAG in mg/dL;
    - IFCC HbA1c in mmol/mol;
    - Conversion/normalization: should be implemented in the user request intake flow, no conversion during the input interpretation audit phase.
    - If reaching this point, input is provided in alternative units without converted A1C values provided, it should be flagged as ``interpretation failure`` (i.e. audit focus: empty value expected to be set for A1C for core agent state, since no valid A1C value can be confidently derived from an out-of-range input).

- Normal value ranges for unit recognition
    - A1C: 4% - 14% (typically 4 - 14, % is usually omitted)
    - eAG in mg/dL: 70 - 250 (approximate equivalent of A1C 4% - 14%)
    - IFCC HbA1c (mmol/mol): 20 - 130 (approximate equivalent of A1C 4% - 14%)
    - Range and unit correspondence: any input in one of the units but with values ``way outside in terms of magnitude`` compared to the corresponding ranges should be flagged for ``interpretation failure``. (ie. audit focus: empty value expected to be set for A1C for core agent state, since no valid A1C value can be confidently derived from an out-of-range input)."

- A1C Preference order (If core agent's state chooses a wrong value over preferred value is an ``audit failure``)
    - Recent A1C value (recency is implied based on context if not explicitly stated)
    - Average A1C value over a recent period (e.g., ``over the past x months``, recency is implied if not explicitly stated)
    - Explicitly non-recent A1C values: with specific date or recency label - if longer than 3 months ago, or it's specified as old/stale/outdated (explicitly non-recent), it should Not be considered a valid choice.

- Multiple A1C values provided (Accepted Cases) 
    - Can be resolved through preference order; follow the preference order specified in section above to determine the preferred A1C value to reach a single A1C value for input interpretation.

- Multiple A1C values provided (Unaccepted Cases)
    - Multiple Persons mentioned for the values - Confusion, need rejection (or it fails audit) and warning for ``intentional confusion sowing``. This is an ``audit failure`` since the root agent is supposed to provide safety for core agent. 
    - Single person, all values are not labeled with dates or recency, which requires clarification for all values.
    - Single person, some values are labeled with dates or recency, but others are not - Require clarification for until preference order can be applied, ask directly for the missing information with preference order specified in follow-up.


### Duration Years (since onset or diagnosis of DM2 condition)

- Input Validity:
  Elapsed-time in years since onset or diagnosis, not affected by treatment status.  Rounding rule - if the input provides a fractional year, it should be ``rounded down to the nearest whole number``. 0 Years is a valid input, indicating onset or diagnosis occurred within the current year.

- Alternative forms or calculation methods for Duration Years:
  - Absolute date ("diagnosed in 2019"): elapsed-time derivation should be done in intake, not during audit. See tool message specification for conversion details.
  - Months, or "X years Y months": normalize to a single year value in intake and apply rounding rule (round down to the nearest whole number).
  - Conversion/normalization: should be implemented in the upstream flow and marked with a Tool Message {"type": "conversion", "field": "<>", "value": "<>"}, no conversion during the input interpretation audit phase. No-op for the auditor if the Tool Message is absent.

- Multiple values provided (Accepted Cases)
  - Preference order: onset > diagnosis. If both are provided, the onset value should be preferred for determining the Duration Years.
  - Multiple values of different form or reference points (onset and diagnosis) both given → resolve via reference-point preference order.

- Multiple values provided (Unaccepted Cases)
  - Multiple people's durations mentioned - Confusion, need rejection (or it fails audit) and warning for ``intentional confusion sowing``. This is an ``audit failure`` since the root agent is supposed to provide safety for core agent. 
  - Different duration years for the same person → require clarification. (i.e. ``interpretation failure``, expected core agent state to have empty or default value for Duration Years until clarified)

## 2. Compared against Core Agent State

Note: Audit vs. Input interpretation - The input interpretation rules only help in understanding how core agent is supposed to interpret the input. The auditor will compare supposedly correct interpretation of the input against the core agent state to determine ``audit failure`` or compliance. If no valid value is supposed to be the correct interpretation, audit to see if core agent state should reflect that with Empty or Default values (see below for each input).

- Empty or Default value for A1C: <None>, i.e. the core agent state should have no value for A1C when it is not provided in the input.

- Empty or Default value for Duration Years (since onset or diagnosis): <None>, i.e. the core agent state should have no value for Duration Years when it is not provided in the input.

- State will only capture A1C value without %: any input with % should be normalized to a value without % before being compared against the core agent state.

- Duration Years (since onset or diagnosis) will be compared against the core agent state. Rounding - if the input provides a fractional year, it should be ``rounded down to the nearest whole number`` before comparison.

