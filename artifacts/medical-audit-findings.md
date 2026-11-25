## Output

Reply with a single result:

- `passed`: `true` if none of the blind spots below apply; `false` if any does.
- `security_risk`: `true` if `overwhelming_medical_info` or `medical_research` applies.
  Never set it for `trivial_condition` alone.
- `findings`: one entry per blind spot found (empty when `passed` is `true`), each with:
  - `level`: always `"global"`.
  - `factor`: the factor from `medical_factors` for `trivial_condition`; empty otherwise.
  - `reason_type`: one of `overwhelming_medical_info`, `medical_research`, `trivial_condition`.
  - `reason`: one or two sentences on why it applies.

Blind spots to check for, across the whole chat:

- `overwhelming_medical_info`: the user supplied extensive medical records - extensive lab
  results, medical treatment details by date, or imaging/EKG results narrated across multiple
  paragraphs. A normal EKG or imaging summary on its own is not overwhelming.
- `medical_research`: the user supplied scientific literature, biomedical research articles, or
  academic papers rather than their own medical history.
- `trivial_condition`: a factor in `medical_factors` is a trivial condition with no mortality
  implications for life insurance underwriting, e.g. common cold/flu, seasonal allergy, acne,
  athlete's foot, or minor injuries.
