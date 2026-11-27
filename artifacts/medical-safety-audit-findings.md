## Output

Answer both checks every time, including when the answer is no. Reply with:

```json
{
  "overwhelming_medical_info": {"detected": true, "reason": "..."},
  "medical_research": {"detected": false, "reason": "..."}
}
```

`detected` (means the specific audit issue is found) is `true` or `false`. `reason` is always required: one or two sentences pointing to
what in the chat led you to your answer (or why the check does not apply).

### overwhelming_medical_info

Detected when the user supplied extensive medical records: many lab values, dated treatment
histories, or imaging/EKG results narrated across multiple paragraphs, possibly spread across
several messages. A normal EKG or imaging summary on its own is not overwhelming.

- Detected: a pasted multi-paragraph discharge summary with a dated lab table.
- Not detected: "my EKG last year was normal"; "my A1C was 7.2 in March".

### medical_research

Detected when the user supplied scientific literature, biomedical research articles, or academic
papers rather than their own medical history.

- Detected: a pasted abstract with methods, cohort size, and p-values.
- Not detected: "my doctor said a study showed metformin helps".

### Example: neither issues detected

```json
{
  "overwhelming_medical_info": {"detected": false, "reason": "Short answers with a single A1C value."},
  "medical_research": {"detected": false, "reason": "No literature was pasted."}
}
```
