package pathx

// LexicallyNormalize is adapted from the Go standard library's
// filepath.Clean implementation in go/src/internal/filepathlite/path.go,
// which in turn follows Rob Pike, “Lexical File Names in Plan 9 or Getting
// Dot-Dot Right,” https://9p.io/sys/doc/lexnames.html.
//
// The only intentional behavior difference here is that trailing path
// separators are preserved except for ".", "..", and roots.
import (
	"path/filepath"
	"runtime"
)

// LexicallyNormalize is like [filepath.Clean], but preserves a trailing path
// separator when the normalized path is neither "." nor ".." and is not a
// root path.
func LexicallyNormalize(path string) string {
	originalPath := path
	volumeLen := len(filepath.VolumeName(path))
	path = path[volumeLen:]
	if path == "" {
		if volumeLen > 1 && IsPathSeparator(originalPath[0]) && IsPathSeparator(originalPath[1]) {
			return filepath.FromSlash(originalPath)
		}
		return originalPath + "."
	}
	rooted := IsPathSeparator(path[0])

	// Invariants:
	//   - We read from path using readPos.
	//   - We write the normalized path into output using output.writePos.
	//   - dotDotLimit is the earliest output position that a .. element may
	//     remove, either because of a leading separator or because we are
	//     preserving a leading ../../.. prefix.
	inputLen := len(path)
	output := lexicalBuffer{
		pathWithoutVolume: path,
		buffer:            nil,
		writePos:          0,
		originalPath:      originalPath,
		volumeLen:         volumeLen,
	}
	readPos, dotDotLimit := 0, 0
	if rooted {
		output.appendByte(filepath.Separator)
		readPos, dotDotLimit = 1, 1
	}

	for readPos < inputLen {
		switch {
		case IsPathSeparator(path[readPos]):
			// Empty path element.
			readPos++
		case path[readPos] == '.' && (readPos+1 == inputLen || IsPathSeparator(path[readPos+1])):
			// . path element.
			readPos++
		case path[readPos] == '.' && path[readPos+1] == '.' && (readPos+2 == inputLen || IsPathSeparator(path[readPos+2])):
			// .. path element: remove the previous real element when possible.
			readPos += 2
			switch {
			case output.writePos > dotDotLimit:
				output.removeLastPathElement(dotDotLimit)
			case !rooted:
				if output.writePos > 0 {
					output.appendByte(filepath.Separator)
				}
				output.appendByte('.')
				output.appendByte('.')
				dotDotLimit = output.writePos
			}
		default:
			// Real path element.
			if (rooted && output.writePos != 1) || (!rooted && output.writePos != 0) {
				output.appendByte(filepath.Separator)
			}
			for ; readPos < inputLen && !IsPathSeparator(path[readPos]); readPos++ {
				output.appendByte(path[readPos])
			}
		}
	}

	if output.writePos == 0 {
		output.appendByte('.')
	}

	postLexicallyNormalize(&output)
	if hasTrailingPathSeparator(path) && shouldPreserveTrailingSeparator(&output, rooted) {
		output.appendByte(filepath.Separator)
	}
	return filepath.FromSlash(output.string())
}

type lexicalBuffer struct {
	pathWithoutVolume string
	buffer            []byte
	writePos          int
	originalPath      string
	volumeLen         int
}

func (b *lexicalBuffer) byteAt(index int) byte {
	if b.buffer != nil {
		return b.buffer[index]
	}
	return b.pathWithoutVolume[index]
}

func (b *lexicalBuffer) appendByte(c byte) {
	if b.buffer == nil {
		if b.writePos < len(b.pathWithoutVolume) && b.pathWithoutVolume[b.writePos] == c {
			b.writePos++
			return
		}
		b.buffer = make([]byte, len(b.pathWithoutVolume))
		copy(b.buffer, b.pathWithoutVolume[:b.writePos])
	}
	b.buffer[b.writePos] = c
	b.writePos++
}

func (b *lexicalBuffer) prependBytes(prefix ...byte) {
	newBuffer := make([]byte, len(prefix)+b.writePos)
	copy(newBuffer, prefix)
	copy(newBuffer[len(prefix):], b.buffer[:b.writePos])
	b.buffer = newBuffer
	b.writePos += len(prefix)
}

func (b *lexicalBuffer) removeLastPathElement(stopPos int) {
	b.writePos--
	for b.writePos > stopPos && !IsPathSeparator(b.byteAt(b.writePos)) {
		b.writePos--
	}
}

func (b *lexicalBuffer) string() string {
	if b.buffer == nil {
		return b.originalPath[:b.volumeLen+b.writePos]
	}
	return b.originalPath[:b.volumeLen] + string(b.buffer[:b.writePos])
}

func shouldPreserveTrailingSeparator(output *lexicalBuffer, rooted bool) bool {
	if output.writePos == 0 {
		return false
	}
	if rooted && output.writePos == 1 {
		return false
	}
	if output.writePos == 1 && output.byteAt(0) == '.' {
		return false
	}
	if output.writePos == 2 && output.byteAt(0) == '.' && output.byteAt(1) == '.' {
		return false
	}
	return !IsPathSeparator(output.byteAt(output.writePos - 1))
}

func postLexicallyNormalize(output *lexicalBuffer) {
	if runtime.GOOS != "windows" || output.volumeLen != 0 || output.buffer == nil {
		return
	}
	for _, c := range output.buffer[:output.writePos] {
		if IsPathSeparator(c) {
			break
		}
		if c == ':' {
			output.prependBytes('.', filepath.Separator)
			return
		}
	}
	if output.writePos >= 3 && IsPathSeparator(output.buffer[0]) && output.buffer[1] == '?' && output.buffer[2] == '?' {
		output.prependBytes(filepath.Separator, '.')
	}
}

func hasTrailingPathSeparator(path string) bool {
	if path == "" {
		return false
	}
	return IsPathSeparator(path[len(path)-1])
}

func IsPathSeparator(c byte) bool {
	if runtime.GOOS == "windows" {
		return c == '\\' || c == '/'
	}
	return c == filepath.Separator
}
