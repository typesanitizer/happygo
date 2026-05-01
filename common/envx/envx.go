// Package envx provides an immutable environment mapping.
package envx

import (
	"iter"
	"runtime"
	"strings"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/collections"
	"github.com/typesanitizer/happygo/common/core/op"
	"github.com/typesanitizer/happygo/common/core/option"
	"github.com/typesanitizer/happygo/common/core/pair"
)

// Env is an immutable environment mapping.
//
// Keys are canonicalized before insertion and lookup.
//
// On Windows, envx canonicalizes keys with strings.ToUpper. This assumes Go's
// case conversion is equivalent to the behavior Windows uses for environment
// variable names; see NOTE(id: windows-envvar-canonicalization).
type Env struct {
	entries collections.MonotoneMap[string, envEntry]
}

type envEntry struct {
	// key preserves the most recently inserted spelling for Entries().
	key string
	// value is the value paired with key.
	value string
}

func Empty() Env {
	return Env{entries: collections.NewMonotoneMap[string, envEntry]()}
}

// New creates a new Env with the given key-value pairs.
//
// Pre-condition: The keys should already be canonicalized per OS conventions.
// E.g. on Windows, both PATH and path cannot be map keys.
func New(kvs iter.Seq[pair.KeyValue[string, string]]) Env {
	entries := collections.NewMonotoneMap[string, envEntry]()
	for kv := range kvs {
		ck := canonicalKey(kv.Key)
		switch entries.InsertOrKeep(ck, envEntry{key: kv.Key, value: kv.Value}) {
		case op.InsertedNew:
			continue
		case op.KeptOld:
			oldKey := entries.Lookup(ck).Unwrap().key
			assert.Preconditionf(false,
				"argument has keys %q and %q, both of which canonicalize to %q",
				oldKey, kv.Key, ck)
		}
	}
	return Env{entries}
}

// NewIgnoringDupes creates a new Env with the given key-value pairs.
//
// Unlike New, if two keys are canonicalized the same way, the later one
// will be preferred.
func NewIgnoringDupes(kvs iter.Seq[pair.KeyValue[string, string]]) Env {
	entries := collections.NewMonotoneMap[string, envEntry]()
	for kv := range kvs {
		ck := canonicalKey(kv.Key)
		entries.InsertOrReplace(ck, envEntry{key: kv.Key, value: kv.Value})
	}
	return Env{entries}
}

// Lookup returns the value for key, if present.
//
// Expected time: Θ(1).
func (env Env) Lookup(key string) option.Option[string] {
	entry, ok := env.entries.Lookup(canonicalKey(key)).Get()
	if !ok {
		return option.None[string]()
	}
	return option.Some(entry.value)
}

// Entries returns the environment as "key=value" strings, similar to
// os.Environ().
//
// Each returned key-value pair uses the most recently inserted spelling for
// its canonical key.
//
// Time: Θ(|env|). Additional space: Θ(|env|).
func (env Env) Entries() []string {
	entries := make([]string, 0, env.entries.Len())
	for key := range env.entries.Keys() {
		entry, ok := env.entries.Lookup(key).Get()
		assert.Invariantf(ok, "monotone env missing key %q during enumeration", key)
		entries = append(entries, entry.key+"="+entry.value)
	}
	return entries
}

// InsertOrKeep inserts the key-value pair if the canonicalized key is absent.
//
// Time: Θ(1) if the key is already present. Otherwise, Θ(|env|).
// Additional space: Θ(1) if the key is already present. Otherwise, Θ(|env|).
func (env Env) InsertOrKeep(key string, value string) (Env, op.InsertResult) {
	canonical := canonicalKey(key)
	if env.entries.Lookup(canonical).IsSome() {
		return env, op.KeptOld
	}
	next := env.entries.CloneWithout(collections.NewSet[string]())
	res := next.InsertOrKeep(canonical, envEntry{key: key, value: value})
	assert.Invariantf(res == op.InsertedNew, "cloned env unexpectedly kept key %q", canonical)
	return Env{entries: next}, res
}

// InsertOrReplace inserts or replaces the key-value pair, returning the old
// value if a canonicalized key was already present.
//
// If only the stored key spelling changes, the returned Env still records the
// new spelling so Entries() reflects the latest inserted key.
//
// Time: Θ(1) if env already stores the exact key-value pair. Otherwise,
// Θ(|env|).
// Additional space: Θ(1) if env already stores the exact key-value pair.
// Otherwise, Θ(|env|).
func (env Env) InsertOrReplace(key string, value string) (Env, option.Option[string]) {
	canonical := canonicalKey(key)
	old, hadOld := env.entries.Lookup(canonical).Get()
	// Preserve the latest inserted key spelling for Entries(). If only the
	// spelling changes under canonicalization, we still need to replace.
	if hadOld && old.key == key && old.value == value {
		return env, option.Some(old.value)
	}

	next := env.entries.CloneWithout(collections.NewSet[string]())
	prev, ok := next.InsertOrReplace(canonical, envEntry{key: key, value: value}).Get()
	if !ok {
		return Env{entries: next}, option.None[string]()
	}
	return Env{entries: next}, option.Some(prev.value)
}

// CloneWithout returns a shallow clone of env with keys in omit removed, if
// present.
//
// Time: Θ(1) if omit is empty. Otherwise, Θ(|env| + |omit|).
// Additional space: Θ(1) if omit is empty. Otherwise, Θ(|env| + |omit|).
//
// Pre-condition: The values in omit should already be canonicalized per OS conventions.
// E.g. on Windows, omit must not contain both "path" and "PATH".
func (env Env) CloneWithout(omit collections.Set[string]) Env {
	if omit.Len() == 0 {
		return env
	}
	canonicalOmit := collections.NewSet[string]()
	for key := range omit.ValuesNonDet() {
		canonical := canonicalKey(key)
		if env.entries.Lookup(canonical).IsSome() {
			canonicalOmit.InsertNew(canonical)
		}
	}
	if canonicalOmit.Len() == 0 {
		return env
	}
	return Env{entries: env.entries.CloneWithout(canonicalOmit)}
}

func canonicalKey(key string) string {
	if runtime.GOOS != "windows" {
		return key
	}
	return strings.ToUpper(key)
}
