## Output

Audit the core agent's recorded state for this factor. Reply with a single result:

- `factor`: the factor exactly as given.
- `passed`: `true` if you agree with the core agent's state; `false` if you disagree.
- `conflict_kind`: the most serious conflict you found in the chat, one of
  `none`, `multiple_people`, `unlabeled_values`, `stale_value`, `differing_values`, `other`.
- `reason`: one or two sentences on why you agree or disagree.
- `findings`: `chat -> <value or none>; state -> <value or none>; agree|disagree`.

Judging clinical significance of a value is out of scope.
