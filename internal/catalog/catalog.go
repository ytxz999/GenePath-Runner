// Package catalog is the single source of truth for the values the API
// accepts. The frontend fetches it instead of hard-coding duplicate lists, so a
// new organism or annotation source only has to be added once.
package catalog

// Organism is one supported species. Code is the g:Profiler organism name.
type Organism struct {
	Code    string `json:"code"`
	Label   string `json:"label"`
	Chinese string `json:"chinese"`
}

// Source is one annotation source accepted by the provider.
type Source struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
}

// Correction is one multiple-testing correction method.
type Correction struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// Organisms lists the species GenePath offers. The list is deliberately short:
// a curated set is easier to validate than "anything g:Profiler supports".
var Organisms = []Organism{
	{Code: "hsapiens", Label: "Homo sapiens", Chinese: "人"},
	{Code: "mmusculus", Label: "Mus musculus", Chinese: "小鼠"},
	{Code: "rnorvegicus", Label: "Rattus norvegicus", Chinese: "大鼠"},
	{Code: "dmelanogaster", Label: "Drosophila melanogaster", Chinese: "果蝇"},
	{Code: "scevisiae", Label: "Saccharomyces cerevisiae", Chinese: "酿酒酵母"},
	{Code: "athaliana", Label: "Arabidopsis thaliana", Chinese: "拟南芥"},
	{Code: "drerio", Label: "Danio rerio", Chinese: "斑马鱼"},
}

// Sources are the annotation databases exposed in the parameter panel.
var Sources = []Source{
	{Code: "GO:BP", Label: "GO Biological Process", Hint: "生物学过程"},
	{Code: "GO:CC", Label: "GO Cellular Component", Hint: "细胞组分"},
	{Code: "GO:MF", Label: "GO Molecular Function", Hint: "分子功能"},
	{Code: "KEGG", Label: "KEGG Pathways", Hint: "代谢与信号通路"},
	{Code: "REAC", Label: "Reactome", Hint: "人工审编通路"},
	{Code: "WP", Label: "WikiPathways", Hint: "社区维护通路"},
}

// Corrections are the accepted multiple-testing correction methods.
var Corrections = []Correction{
	{Code: "g_SCS", Label: "g:SCS（推荐）"},
	{Code: "bonferroni", Label: "Bonferroni"},
	{Code: "fdr", Label: "Benjamini-Hochberg FDR"},
}

// Defaults used when a request omits a field.
const (
	DefaultOrganism     = "hsapiens"
	DefaultSignificance = "0.05"
	DefaultCorrection   = "g_SCS"
	DefaultBackground   = "annotated"
	DefaultSource       = "GO:BP"
)

// OrganismByCode looks up an organism, returning false when unsupported.
func OrganismByCode(code string) (Organism, bool) {
	for _, o := range Organisms {
		if o.Code == code {
			return o, true
		}
	}
	return Organism{}, false
}

// ValidSource reports whether the code is an accepted annotation source.
func ValidSource(code string) bool {
	for _, s := range Sources {
		if s.Code == code {
			return true
		}
	}
	return false
}

// ValidCorrection reports whether the code is an accepted correction method.
func ValidCorrection(code string) bool {
	for _, c := range Corrections {
		if c.Code == code {
			return true
		}
	}
	return false
}

// OrganismLabel returns the display label for an organism code.
func OrganismLabel(code string) string {
	if o, ok := OrganismByCode(code); ok {
		return o.Label
	}
	return code
}
