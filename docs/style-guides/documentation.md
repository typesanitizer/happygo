# Documentation style guide

## Citation style guide

### In-text references

Use Markdown reference-style links with descriptive display text woven into
the prose:
```
The [Microsoft documentation on file path formats][ms-file-path-formats]
defines a traditional DOS path as ...
```

Label naming convention: `author-or-org` + `-` + `short-topic`. Use lowercase,
hyphens only. Examples:
- `liptak-drive-letters`
- `ms-modifypartition-letter`
- `mdn-array-flat`

### References section

Each entry in the `## References` section has:
- Author or organization
- Title, hyperlinked to the canonical URL using the reference label
- Archive link and snapshot date, or access date if no archive exists

Format:
```
- Author, "[Title][label]."
  ([archived][label-a], YYYY-MM-DD)
```

If no archive is available:
```
- Author, "[Title][label]."
  (accessed YYYY-MM-DD)
```

### Link definitions

Place all link definitions at the bottom of the file, grouped by reference.
Use the `-a` suffix for archive URLs:
```
[label]: <canonical URL>
[label-a]: <archive URL>
```

<details>
<summary>Why use named references?</summary>

Numeric indices (e.g. `[1]`, `[2]`) require renumbering when a citation is
inserted in the middle of a document, which creates noisy diffs and risks
references pointing to the wrong source after an edit. Named keys are stable
under insertion and immediately tell the reader what is being cited without
chasing a footnote.
</details>

### Rules

1. Labels must be unique, descriptive, and stable.
2. Every external link must have an entry in the References section.
3. Always provide an archive link when one exists. If you create one, use
   archive.today or the Wayback Machine.
4. Dates are always ISO 8601: `YYYY-MM-DD`.
