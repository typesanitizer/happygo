package winpath

import "strings"

// IsWindowsStyleAbsPath reports whether path uses Windows path syntax where
// Unix-style lexical component walking is not reliable.
//
// Windows path form references:
//   - Drive-qualified paths (absolute like C:\foo and drive-relative like C:foo):
//     https://learn.microsoft.com/en-us/dotnet/standard/io/file-path-formats
//   - UNC and rooted paths (\\server\share\foo, \foo):
//     https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file
func IsWindowsStyleAbsPath(path string) bool {
	if len(path) >= 2 && isASCIIAlpha(path[0]) && path[1] == ':' {
		return true
	}
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, `\`) {
		return true
	}
	if strings.HasPrefix(path, `//`) {
		return true
	}
	return false
}

// Per Windows path format docs, a drive designator is a single alphabetic
// character followed by ':', i.e. limited to ASCII A-Z/a-z.
// https://learn.microsoft.com/en-us/dotnet/standard/io/file-path-formats
func isASCIIAlpha(b byte) bool {
	return 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z'
}
