package board

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// References are the documents a task points at: repository-relative paths, each optionally
// carrying an anchor, such as CONTEXT.md#index.
//
// They are a JSON array in one column rather than a second table, because a reference is a
// name and not an entity. Nothing joins on it, nothing addresses it by id, and the documents
// themselves stay in the repository where git can review them — the alternative, holding them
// in state.db, would make every one of them machine-local and lose them on the next clone.
//
// This is deliberately NOT the Index. prd.md §13 designs a searchable projection of a
// project's prose, and that is a larger thing with its own open questions. A reference is a
// path today and can resolve to an index entry later without this shape changing.

// NormalizeReferences trims, drops blanks and duplicates, and refuses anything that is not
// a path inside this repository.
//
// The refusal is not fussiness. A reference is printed in a file other people read, and a
// reference that walks out of the repository — or reaches an absolute path on the machine that
// happened to write it — is a path that means one thing to its author and nothing at all to
// anyone else. Seraph has already spent a day on files that meant one thing and were read as
// another.
func NormalizeReferences(in []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, raw := range in {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			continue
		}
		path := ReferencePath(ref)
		if path == "" {
			return nil, &Error{Reason: ReasonInvalidReference, Message: fmt.Sprintf("%q has no file part", ref)}
		}
		if filepath.IsAbs(path) {
			return nil, &Error{Reason: ReasonInvalidReference,
				Message: fmt.Sprintf("reference %q must be relative to the repository root", ref)}
		}
		if escapes(path) {
			return nil, &Error{Reason: ReasonInvalidReference,
				Message: fmt.Sprintf("reference %q points outside the repository", ref)}
		}
		if seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	return out, nil
}

// ReferencePath returns the file part of a reference, dropping any anchor.
func ReferencePath(ref string) string {
	path, _, _ := strings.Cut(ref, "#")
	return strings.TrimSpace(path)
}

// escapes reports whether a cleaned relative path climbs out of the repository.
func escapes(path string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(filepath.Clean(path)), "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

// encodeReferences renders the list for storage. An empty list is stored as NULL rather
// than as "[]", so a column scan cannot tell a task with no references from one whose
// references were never set.
func encodeReferences(refs []string) any {
	if len(refs) == 0 {
		return nil
	}
	encoded, err := json.Marshal(refs)
	if err != nil {
		return nil
	}
	return string(encoded)
}

// decodeReferences reads the stored column back. Anything unreadable is treated as no
// references rather than as an error, because a board that cannot be read is worse than one
// that quietly shows a task without its paperwork.
func decodeReferences(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var refs []string
	if err := json.Unmarshal([]byte(raw), &refs); err != nil {
		return nil
	}
	return refs
}
