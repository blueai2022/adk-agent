## Output

Reply with a single result:

- `passed`: `true` if no factor in `medical_factors` is a trivial condition; `false` if any is.
- `security_risk`: always `false`. Over-detected trivial conditions are not a safety concern.
- `findings`: one entry per trivial condition found (empty when `passed` is `true`), each with:
  - `level`: always `"global"`.
  - `factor`: the factor from `medical_factors` that is trivial.
  - `reason_type`: always `trivial_condition`.
  - `reason`: one or two sentences on why it has no mortality implications for life insurance
    underwriting.

A factor is a `trivial_condition` when it has no mortality implications for life insurance
underwriting, e.g. common cold/flu, seasonal allergy, acne, athlete's foot, or minor injuries.
