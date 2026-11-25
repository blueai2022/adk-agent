## Output

Reply with a single result:

- `passed`: `true` if neither blind spot below applies; `false` if either does.
- `security_risk`: `true` whenever `passed` is `false` - both blind spots below are safety-flagged.
- `findings`: one entry per blind spot found (empty when `passed` is `true`), each with:
  - `level`: always `"global"`.
  - `factor`: always empty - these blind spots are not tied to a specific factor.
  - `reason_type`: one of `overwhelming_medical_info`, `medical_research`.
  - `reason`: one or two sentences on why it applies.

Blind spots to check for, across the full conversation so far:

- `overwhelming_medical_info`: the user supplied extensive medical records - extensive lab
  results, medical treatment details by date, or imaging/EKG results narrated across multiple
  paragraphs, possibly spread across several messages. A normal EKG or imaging summary on its own
  is not overwhelming.
- `medical_research`: the user supplied scientific literature, biomedical research articles, or
  academic papers rather than their own medical history.
