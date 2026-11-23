<!--
Authoring template, not consumed at runtime.

Copy to `<factor-key>-audit-findings.md` (see dm2-audit-findings.md), fill in
every `{{TOKEN}}`, and delete this comment block. The file is appended to the
audit agent's instruction as its Output section, so keep it flat and concrete.
Factors without their own file use default-audit-findings.md.

The reply fields must match AuditResult in audit.go.

Tokens:
  {{SIGNALS}}  signal names in the core agent state, e.g. "`a1c` and `duration` (DM2)"
-->

## Output

Audit the signals {{SIGNALS}}. Reply with a single result:

- `factor`: the factor exactly as given.
- `passed`: `true` if you agree with the core agent's state for every signal; `false` if you disagree with any.
- `conflict_kind`: the most serious conflict you found in the chat, one of
  `none`, `multiple_people`, `unlabeled_values`, `stale_value`, `differing_values`, `other`.
- `reason`: one or two sentences on why you agree or disagree. Name the signal.
- `findings`: one line per signal: `<signal>: chat -> <value or none>; state -> <value or none>; agree|disagree`.

Judging clinical significance of a value is out of scope.
