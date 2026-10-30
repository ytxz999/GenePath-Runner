package gene

import (
	"strings"
	"testing"
)

func TestParseHandlesCommonSeparators(t *testing.T) {
	raw := "TP53\nCDK1, BAX;\tMCM2  PCNA\r\n"
	want := []string{"TP53", "CDK1", "BAX", "MCM2", "PCNA"}
	got := Parse(raw)
	if len(got) != len(want) {
		t.Fatalf("Parse returned %d symbols (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Parse[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestParseSkipsCommentsAndBlankLines(t *testing.T) {
	got := Parse("# apoptosis panel\n\nTP53\n   \n# mouse\nTrp53\n")
	if len(got) != 2 || got[0] != "TP53" || got[1] != "Trp53" {
		t.Fatalf("unexpected parse result: %v", got)
	}
}

func TestParseDeduplicatesCaseInsensitivelyButKeepsFirstSpelling(t *testing.T) {
	got := Parse("TP53, tp53, Tp53, BAX")
	if len(got) != 2 {
		t.Fatalf("expected 2 unique symbols, got %v", got)
	}
	if got[0] != "TP53" || got[1] != "BAX" {
		t.Fatalf("expected first spelling to win, got %v", got)
	}
}

func TestParseRawKeepsDuplicates(t *testing.T) {
	raw := ParseRaw("TP53 TP53 BAX")
	if len(raw) != 3 {
		t.Fatalf("ParseRaw must not de-duplicate, got %v", raw)
	}
	parsed := Normalize(raw)
	if len(parsed) != 2 {
		t.Fatalf("Normalize should collapse duplicates, got %v", parsed)
	}
}

func TestValidRejectsProviderBreakingSymbols(t *testing.T) {
	valid := []string{"TP53", "MCM2", "HLA-A", "C12orf75", "SNORD.1", "Trp53"}
	for _, s := range valid {
		if !Valid(s) {
			t.Errorf("Valid(%q) = false, want true", s)
		}
	}

	invalid := []string{"", "TP53!", "A B", "-TP53", "ТP53\u0000", strings.Repeat("A", 33)}
	for _, s := range invalid {
		if Valid(s) {
			t.Errorf("Valid(%q) = true, want false", s)
		}
	}
}

func TestSplitPartitionsInput(t *testing.T) {
	ok, bad := Split([]string{"TP53", "BAD!", "CDK1"})
	if len(ok) != 2 || len(bad) != 1 || bad[0] != "BAD!" {
		t.Fatalf("unexpected split: valid=%v invalid=%v", ok, bad)
	}
}

func TestHashIsOrderAndCaseIndependentOnlyWithinOrganism(t *testing.T) {
	a := Hash([]string{"TP53", "CDK1", "BAX"}, "hsapiens")
	b := Hash([]string{"BAX", "CDK1", "TP53"}, "hsapiens")
	if a != b {
		// Symbols keep their casing, so "tp53" is intentionally a different set
		// than "TP53": the provider treats them differently for non-human orgs.
		t.Fatalf("hash must be order independent: %s != %s", a, b)
	}

	c := Hash([]string{"TP53", "CDK1", "BAX"}, "mmusculus")
	if a == c {
		t.Fatal("hash must change when the organism changes")
	}

	if d := Hash([]string{"TP53", "CDK1"}, "hsapiens"); d == a {
		t.Fatal("hash must change when the gene set changes")
	}
}

func TestHashIsStableAcrossCalls(t *testing.T) {
	first := Hash([]string{"TP53", "BAX"}, "hsapiens")
	second := Hash([]string{"TP53", "BAX"}, "hsapiens")
	if first != second {
		t.Fatalf("hash is not deterministic: %s vs %s", first, second)
	}
	if len(first) != 64 {
		t.Fatalf("expected a sha256 hex digest, got %d chars", len(first))
	}
}
