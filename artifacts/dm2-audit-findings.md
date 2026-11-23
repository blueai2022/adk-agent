## Output

Audit the signals `a1c` and `duration` (DM2). Reply with a single result:

- `factor`: the factor exactly as given.
- `passed`: `true` if you agree with the core agent's state for every signal; `false` if you disagree with any.
- `conflict_kind`: the most serious conflict you found in the chat, one of
  `none`, `multiple_people`, `unlabeled_values`, `stale_value`, `differing_values`, `other`.
- `reason`: one or two sentences on why you agree or disagree. Name the signal.
- `findings`: one line per signal: `<signal>: chat -> <value or none>; state -> <value or none>; agree|disagree`.
  Add a short note when you resolved several values using the preference order.

Judging clinical significance of a value is out of scope.
