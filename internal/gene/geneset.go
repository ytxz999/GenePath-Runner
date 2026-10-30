// Package gene holds the pure functions that turn messy user input into a
// canonical gene set. Keeping this logic in its own package means the HTTP
// layer, the seeder and the future executor all agree on what "the same input"
// means — which is the whole point of the reproducibility hash.
package gene

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

// MaxGenes is the hard cap enforced before any provider call is made.
const MaxGenes = 2000

// Parse splits free-form text into a canonical, de-duplicated gene list.
func Parse(raw string) []string {
	return Normalize(ParseRaw(raw))
}

// ParseRaw splits free-form text into candidate tokens without de-duplicating
// them. Callers use the difference between ParseRaw and Parse to report how
// many duplicate symbols the user pasted.
func ParseRaw(raw string) []string {
	// A leading '#' marks a comment line: users pasting a panel often keep the
	// header lines from a spreadsheet, and gene symbols never contain '#'.
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = line[:idx]
		}
		for _, f := range strings.FieldsFunc(line, func(r rune) bool {
			return r == '\r' || r == ',' || r == ';' || r == '\t' || r == ' '
		}) {
			f = strings.TrimSpace(f)
			if f != "" {
				out = append(out, f)
			}
		}
	}
	return out
}

// Normalize trims and de-duplicates symbols while preserving the order and the
// spelling in which they first appeared.
//
// Casing is deliberately NOT rewritten: human symbols are upper-case (TP53) but
// mouse symbols are title-case (Trp53), so silently up-casing would corrupt
// every non-human submission. De-duplication is case-insensitive, which is what
// users expect when they paste a list that mixes "tp53" and "TP53".
func Normalize(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		s := strings.TrimSpace(item)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

// Valid reports whether a normalized symbol is plausible enough to send
// upstream. It intentionally rejects characters that would break the provider
// request rather than silently rewriting them.
func Valid(symbol string) bool {
	if symbol == "" || len(symbol) > 32 {
		return false
	}
	for i, r := range symbol {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
		case r == '-' || r == '_' || r == '.':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// Split partitions symbols into accepted and rejected slices.
func Split(items []string) (valid, invalid []string) {
	for _, item := range items {
		if Valid(item) {
			valid = append(valid, item)
		} else {
			invalid = append(invalid, item)
		}
	}
	return valid, invalid
}

// Hash returns the stable content hash of a gene set for a given organism.
//
// The hash is order-independent and case-independent: sorting a copy before
// hashing means "TP53, CDK1" and "cdk1, tp53" collapse to the same run
// identity, so the UI can warn about duplicate submissions and the cache can
// reuse a previous response.
func Hash(symbols []string, organism string) string {
	sorted := make([]string, len(symbols))
	for i, s := range symbols {
		// Upper-casing before hashing keeps the identity case-insensitive,
		// which is how de-duplication already treats symbols. Upper- rather
		// than lower-case is deliberate: every human symbol is already upper
		// case, so hashes recorded before this normalisation existed keep
		// their value instead of silently changing under stored runs.
		sorted[i] = strings.ToUpper(strings.TrimSpace(s))
	}
	sortStrings(sorted)

	sum := sha256.New()
	sum.Write([]byte(organism))
	sum.Write([]byte{0})
	sum.Write([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum.Sum(nil))
}

// sortStrings is a tiny insertion sort: gene lists are small, and this keeps
// the package dependency-free.
func sortStrings(items []string) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j] < items[j-1]; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
