## Output

Answer for every factor in `medical_factors`, including those that are not trivial. Reply with
one entry per factor:

```json
{
  "factors": [
    {"factor": "J00", "trivial": true, "reason": "..."},
    {"factor": "E11.9", "trivial": false, "reason": "..."}
  ]
}
```

- `factor`: the ICD-10 code exactly as it appears in `medical_factors`.
- `trivial`: `true` or `false`.
- `reason`: one or two sentences on why the condition does or does not carry mortality
  implications for life insurance underwriting.

A factor is trivial when it has no mortality implications for life insurance underwriting, e.g.
common cold/flu, seasonal allergy, acne, athlete's foot, or minor injuries. Do not skip a factor
you are unsure about: answer it, and say why in `reason`.

### Example

Input `medical_factors`: `{"J00": "acute nasopharyngitis (common cold)", "E11.9": "type 2 diabetes"}`

```json
{
  "factors": [
    {"factor": "J00", "trivial": true, "reason": "A common cold has no mortality implications."},
    {"factor": "E11.9", "trivial": false, "reason": "Diabetes materially affects mortality risk."}
  ]
}
```
