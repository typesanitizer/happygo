# Windows

## File paths

### Drive letters

The [Microsoft documentation on file path formats][ms-file-path-formats]
defines a traditional DOS path as starting with "a volume or drive letter
followed by the volume separator (`:`)," but does not specify which characters
are valid as drive letters.

Per the [Microsoft documentation on settings for unattended Windows deployment][ms-modifypartition-letter],
*Drive_letter* is "an uppercase letter, C through Z." Drive letters A and B are [historically reserved
for floppy drives][wp-drive-letter-assignment] but are otherwise valid. Drive letters are
case-insensitive per the [Naming Files documentation][ms-naming-files]:
`C:` and `c:` refer to the same volume. Taken together, one would expect
the full set of valid drive letters to be a-z and A-Z.

However, at the NT kernel level this restriction is not enforced. [Liptak][liptak-drive-letters] documents that
`RtlDosPathNameToNtPathName_U` maps any `X:` prefix to `\??\X:` regardless
of what `X` is:

> Since `RtlDosPathNameToNtPathName_U` converts `C:\foo` to `\??\C:\foo`,
> then an object named `C:` will behave like a drive letter.

Tool support for non-ASCII drive letters is inconsistent: per
[Liptak][liptak-drive-letters], `subst` and `cmd` handle them, but File Explorer and
PowerShell do not. The only hard constraint is that drive letters are
restricted to a single WTF-16 code unit (u16, so <= U+FFFF).

Not all absolute paths on Windows start with a drive letter. Windows also
has UNC paths (`\\server\share\...`) and rooted paths (`\foo`); see the
[file path formats documentation][ms-file-path-formats] for the full
taxonomy.

#### Policy

In the happygo monorepo, we assume drive letters are ASCII letters (A-Z,
case-insensitive). While users can technically assign non-ASCII drive letters
via NT APIs, this is rare enough that we do not handle it.

## Environment variables and executable lookup

### Environment variable names

The [Changing Environment Variables documentation][ms-changing-environment-variables],
in its discussion of Windows environment blocks, states:

> All strings in the environment block must be sorted alphabetically by name.
> The sort is case-insensitive, Unicode order, without regard to locale.

This is the closest Windows-native documentation we have found for the
case-insensitivity of environment variable names. There are indirect
mentions of it in the PowerShell and .NET docs, but no clear Windows
docs on the exact definition of case-insensitivity used.

#### Operating assumptions

<!-- NOTE(id: windows-envvar-canonicalization) -->
We assume that Go's `strings.ToUpper` which is documented to
"return \[the input\] with all Unicode letters mapped to their
upper case." as being equivalent to what Windows needs.
It's technically possible that Windows has a different definition
of casing (even "without regard to locale"), but we haven't been
able to find more authoritative documentation on the definition
of case-insensitivity here.

## References

- Microsoft, "[File path formats on Windows systems][ms-file-path-formats]."
  ([archived][ms-file-path-formats-a], 2026-03-29)

- Microsoft, "[ModifyPartition > Letter][ms-modifypartition-letter]."
  ([archived][ms-modifypartition-letter-a], 2026-03-29)

- Microsoft, "[Naming Files, Paths, and Namespaces][ms-naming-files]."
  ([archived][ms-naming-files-a], 2026-03-27)

- Microsoft, "[Changing Environment Variables][ms-changing-environment-variables]."
  ([archived][ms-changing-environment-variables-a], 2024-09-10)

- Ryan Liptak, "[Windows drive letters are not limited to
  A-Z][liptak-drive-letters]."
  ([archived][liptak-drive-letters-a], 2026-03-27)

- Wikipedia, "[Drive letter assignment][wp-drive-letter-assignment]."
  (accessed 2026-03-29)

[ms-file-path-formats]: <https://learn.microsoft.com/en-us/dotnet/standard/io/file-path-formats>
[ms-file-path-formats-a]: <https://web.archive.org/web/20260329121937/https://learn.microsoft.com/en-us/dotnet/standard/io/file-path-formats>
[ms-modifypartition-letter]: <https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/microsoft-windows-setup-diskconfiguration-disk-modifypartitions-modifypartition-letter>
[ms-modifypartition-letter-a]: <https://web.archive.org/web/20260329122013/https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/microsoft-windows-setup-diskconfiguration-disk-modifypartitions-modifypartition-letter>
[ms-naming-files]: <https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file>
[ms-naming-files-a]: <https://archive.is/H34KB>
[ms-changing-environment-variables]: <https://learn.microsoft.com/en-us/windows/win32/procthread/changing-environment-variables>
[ms-changing-environment-variables-a]: <https://web.archive.org/web/20240910051217/https://learn.microsoft.com/en-us/windows/win32/procthread/changing-environment-variables>
[wp-drive-letter-assignment]: <https://en.wikipedia.org/wiki/Drive_letter_assignment>
[liptak-drive-letters]: <https://www.ryanliptak.com/blog/windows-drive-letters-are-not-limited-to-a-z/>
[liptak-drive-letters-a]: <https://archive.is/Scqo0>
