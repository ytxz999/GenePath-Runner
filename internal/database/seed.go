package database

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"genepath/internal/gene"
	"genepath/internal/model"
)

// This file is the ONLY place demo data lives. Every row created here is marked
// IsDemo, the UI badges those rows, and the generated report prints a warning.
// Nothing here may be mistaken for a real analysis run.

// DemoDataVersion is the annotation release shown on demo rows. It is a fixed
// string, not a live value, because demo rows are not produced by a real call.
const DemoDataVersion = "e114_eg62_p19 (演示)"

var cellCycleGenes = []string{
	"CDK1", "CDK2", "CCNB1", "CCNB2", "CCNE1", "CCNA2", "MCM2", "MCM3", "MCM4", "MCM7",
	"PCNA", "RFC2", "AURKA", "AURKB", "PLK1", "BUB1", "BUB1B", "MAD2L1", "TOP2A", "CENPE",
	"KIF11", "TP53", "CDKN1A", "E2F1",
}

var apoptosisGenes = []string{
	"TP53", "BAX", "BAK1", "BCL2", "BCL2L1", "CASP3", "CASP8", "CASP9", "APAF1",
	"CYCS", "DIABLO", "XIAP", "FAS", "FADD", "BID", "PMAIP1", "BBC3", "CDKN1A",
}

var immuneGenes = []string{
	"CD3D", "CD3E", "CD28", "ICOS", "IL2", "IL2RA", "IL2RB", "IL7R", "JAK1", "JAK3",
	"STAT5A", "STAT5B", "LCK", "ZAP70", "LAT", "PLCG1", "PRKCQ", "CARD11", "NFKB1",
	"RELA", "NFATC1", "NFATC2", "CCR7", "SELL", "LTB", "TNFRSF4",
}

var repairGenes = []string{
	"BRCA1", "BRCA2", "RAD51", "RAD51C", "PALB2", "ATM", "ATR", "CHEK1", "CHEK2",
	"XRCC1", "LIG1", "MSH2", "MSH6", "MLH1", "PCNA", "RPA1",
}

// termSeed is one enrichment row before it is turned into a model.Term.
type termSeed struct {
	source string
	native string
	name   string
	p      float64
	size   int
	genes  []string
}

var cellCycleTerms = []termSeed{
	{"GO:BP", "GO:0007049", "cell cycle", 1.2e-18, 1024, []string{"CDK1", "CCNB1", "CDK2", "CCNE1", "MCM2", "PCNA", "AURKA", "PLK1"}},
	{"GO:BP", "GO:0000278", "mitotic cell cycle", 3.4e-16, 780, []string{"CDK1", "CCNB1", "AURKB", "BUB1", "PLK1", "TOP2A"}},
	{"GO:BP", "GO:0006260", "DNA replication", 8.9e-14, 218, []string{"MCM2", "MCM3", "MCM4", "MCM7", "PCNA", "RFC2"}},
	{"GO:BP", "GO:0000086", "G2/M transition of mitotic cell cycle", 2.1e-12, 176, []string{"CDK1", "CCNB1", "CCNB2", "PLK1"}},
	{"GO:BP", "GO:0007059", "chromosome segregation", 6.7e-11, 240, []string{"BUB1", "BUB1B", "MAD2L1", "CENPE", "AURKB"}},
	{"GO:BP", "GO:0000070", "mitotic sister chromatid segregation", 1.4e-9, 132, []string{"BUB1", "MAD2L1", "CENPE"}},
	{"GO:BP", "GO:0007051", "spindle organization", 4.8e-8, 164, []string{"AURKA", "PLK1", "KIF11", "CENPE"}},
	{"GO:BP", "GO:0031570", "DNA integrity checkpoint signaling", 9.3e-7, 96, []string{"TP53", "CDKN1A", "BUB1"}},
	{"KEGG", "hsa04110", "Cell cycle", 5.2e-15, 124, []string{"CDK1", "CDK2", "CCNB1", "CCNE1", "MCM2", "PCNA", "TP53"}},
	{"KEGG", "hsa04115", "p53 signaling pathway", 3.8e-9, 68, []string{"TP53", "CDKN1A", "CCNB1"}},
	{"REAC", "R-HSA-69278", "Cell Cycle, Mitotic", 2.7e-17, 620, []string{"CDK1", "CCNB1", "AURKA", "PLK1", "BUB1", "TOP2A"}},
	{"REAC", "R-HSA-69304", "DNA Replication", 5.5e-12, 178, []string{"MCM2", "MCM3", "MCM4", "PCNA", "RFC2"}},
}

var apoptosisTerms = []termSeed{
	{"GO:BP", "GO:0006915", "apoptotic process", 4.1e-16, 890, []string{"BAX", "BAK1", "CASP3", "CASP9", "APAF1", "CYCS", "BID"}},
	{"GO:BP", "GO:0097190", "apoptotic signaling pathway", 7.2e-13, 410, []string{"FAS", "FADD", "BID", "CASP8", "TP53"}},
	{"GO:BP", "GO:0043065", "positive regulation of apoptotic process", 2.8e-11, 520, []string{"TP53", "BBC3", "PMAIP1", "BAX", "BAK1"}},
	{"GO:BP", "GO:0008629", "intrinsic apoptotic signaling pathway", 5.4e-10, 180, []string{"BAX", "BAK1", "CYCS", "APAF1", "DIABLO"}},
	{"GO:BP", "GO:0008630", "intrinsic apoptotic signaling pathway in response to DNA damage", 3.3e-7, 88, []string{"TP53", "PMAIP1", "BBC3", "BAX"}},
	{"GO:BP", "GO:0043066", "negative regulation of apoptotic process", 1.9e-8, 460, []string{"BCL2", "BCL2L1", "XIAP"}},
	{"KEGG", "hsa04210", "Apoptosis", 6.6e-14, 136, []string{"BAX", "BAK1", "CASP3", "CASP8", "CASP9", "APAF1", "CYCS"}},
	{"KEGG", "hsa04115", "p53 signaling pathway", 2.4e-9, 68, []string{"TP53", "BAX", "PMAIP1", "BBC3", "CDKN1A"}},
	{"REAC", "R-HSA-75153", "Apoptotic execution phase", 1.1e-11, 96, []string{"CASP3", "CASP8", "DIABLO", "XIAP"}},
	{"REAC", "R-HSA-109606", "Intrinsic Pathway for Apoptosis", 8.7e-10, 74, []string{"BAX", "BAK1", "CYCS", "APAF1", "CASP9"}},
}

var immuneTerms = []termSeed{
	{"GO:BP", "GO:0042110", "T cell activation", 2.6e-18, 420, []string{"CD3D", "CD3E", "CD28", "ICOS", "LCK", "ZAP70", "LAT", "PLCG1"}},
	{"GO:BP", "GO:0006955", "immune response", 8.3e-15, 1180, []string{"CD3D", "CD3E", "IL2", "IL2RA", "JAK1", "STAT5A", "LCK", "ZAP70"}},
	{"GO:BP", "GO:0050852", "T cell receptor signaling pathway", 3.7e-13, 210, []string{"CD3D", "CD3E", "LCK", "ZAP70", "LAT", "PLCG1", "PRKCQ"}},
	{"GO:BP", "GO:0031295", "T cell costimulation", 4.9e-12, 130, []string{"CD28", "ICOS", "LCK", "PLCG1", "PRKCQ"}},
	{"GO:BP", "GO:0038111", "interleukin-7-mediated signaling pathway", 9.1e-10, 44, []string{"IL7R", "JAK1", "JAK3", "STAT5A"}},
	{"GO:BP", "GO:0002286", "T cell activation involved in immune response", 6.2e-9, 160, []string{"CD3E", "CD28", "LCK", "ZAP70"}},
	{"KEGG", "hsa04660", "T cell receptor signaling pathway", 5.8e-14, 104, []string{"CD3D", "CD3E", "CD28", "LCK", "ZAP70", "LAT", "PLCG1"}},
	{"KEGG", "hsa04658", "Th1 and Th2 cell differentiation", 2.2e-11, 92, []string{"JAK1", "STAT5A", "NFKB1", "RELA", "NFATC1"}},
	{"KEGG", "hsa04060", "Cytokine-cytokine receptor interaction", 1.3e-8, 295, []string{"IL2", "IL2RA", "IL2RB", "IL7R", "LTB", "TNFRSF4"}},
	{"REAC", "R-HSA-202403", "TCR signaling", 7.4e-13, 138, []string{"CD3D", "CD3E", "LCK", "ZAP70", "LAT", "PLCG1"}},
	{"REAC", "R-HSA-9020591", "Interleukin-2 signaling", 3.1e-10, 76, []string{"IL2", "IL2RA", "IL2RB", "JAK1", "JAK3", "STAT5A"}},
}

var repairTerms = []termSeed{
	{"GO:BP", "GO:0006281", "DNA repair", 1.7e-17, 520, []string{"BRCA1", "BRCA2", "RAD51", "PALB2", "ATM", "ATR", "XRCC1", "MSH2"}},
	{"GO:BP", "GO:0000724", "double-strand break repair via homologous recombination", 3.1e-14, 120, []string{"BRCA1", "BRCA2", "RAD51", "RAD51C", "PALB2"}},
	{"GO:BP", "GO:0006298", "mismatch repair", 8.8e-12, 44, []string{"MSH2", "MSH6", "MLH1", "PCNA", "RPA1"}},
	{"GO:BP", "GO:0000077", "DNA damage checkpoint signaling", 2.4e-10, 148, []string{"ATM", "ATR", "CHEK1", "CHEK2", "BRCA1"}},
	{"GO:BP", "GO:0006302", "double-strand break repair", 5.6e-9, 210, []string{"BRCA1", "RAD51", "ATM", "PALB2"}},
	{"KEGG", "hsa03440", "Homologous recombination", 9.9e-13, 78, []string{"BRCA1", "BRCA2", "RAD51", "RAD51C", "PALB2"}},
	{"KEGG", "hsa03430", "Mismatch repair", 4.3e-10, 23, []string{"MSH2", "MSH6", "MLH1", "PCNA", "RPA1"}},
	{"REAC", "R-HSA-73884", "DNA Damage Recognition and Repair", 6.1e-11, 240, []string{"ATM", "ATR", "CHEK1", "XRCC1", "LIG1"}},
	{"REAC", "R-HSA-5693565", "Recruitment and ATM-mediated phosphorylation of repair proteins", 2.9e-8, 62, []string{"ATM", "BRCA1", "RAD51", "CHEK2"}},
}

// fillerBatches labels the extra history rows that give the activity chart a
// realistic shape. Every extra row reuses one of the curated scenarios below so
// that a "succeeded" row always comes with stored enrichment terms.
var fillerBatches = []string{"复测 1", "复测 2", "批次 3", "复现实验", "补充分析"}

type demoRun struct {
	id       string
	label    string
	status   string
	organism string
	genes    []string
	sources  []string
	created  time.Time
	duration time.Duration
	errMsg   string
	terms    []termSeed
}

// SeedDemo inserts the demo dataset once. Re-running the server is a no-op.
func SeedDemo(db *gorm.DB) error {
	var existing int64
	if err := db.Model(&model.Run{}).Count(&existing).Error; err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}

	runs, terms, events := buildDemoDataset(time.Now())
	return waitForWrite(func() error {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := tx.CreateInBatches(&runs, 50).Error; err != nil {
				return err
			}
			if len(terms) > 0 {
				if err := tx.CreateInBatches(&terms, 100).Error; err != nil {
					return err
				}
			}
			if len(events) > 0 {
				if err := tx.CreateInBatches(&events, 100).Error; err != nil {
					return err
				}
			}
			return nil
		})
	})
}

func buildDemoDataset(now time.Time) ([]model.Run, []model.Term, []model.RunEvent) {
	allSources := []string{"GO:BP", "GO:CC", "GO:MF", "KEGG", "REAC"}

	specs := []demoRun{
		{
			id: "run_7c1a9e4b2f30", label: "Cell cycle regulators — HeLa 同步化样本",
			status: model.StatusSucceeded, organism: "hsapiens", genes: cellCycleGenes,
			sources: allSources, created: now.Add(-2 * time.Hour), duration: 2840 * time.Millisecond,
			terms: cellCycleTerms,
		},
		{
			id: "run_2f8b5d6c1a94", label: "Apoptosis panel — 顺铂处理 24h",
			status: model.StatusSucceeded, organism: "hsapiens", genes: apoptosisGenes,
			sources: []string{"GO:BP", "KEGG", "REAC"}, created: now.Add(-26 * time.Hour), duration: 1420 * time.Millisecond,
			terms: apoptosisTerms,
		},
		{
			id: "run_9d3e71c0aa52", label: "T 细胞活化 signature — PBMC",
			status: model.StatusSucceeded, organism: "hsapiens", genes: immuneGenes,
			sources: allSources, created: now.Add(-2 * 24 * time.Hour), duration: 3310 * time.Millisecond,
			terms: immuneTerms,
		},
		{
			id: "run_5a0c48f2db17", label: "DNA 损伤修复基因集 — 复习整理",
			status: model.StatusSucceeded, organism: "hsapiens", genes: repairGenes,
			sources: []string{"GO:BP", "KEGG", "REAC"}, created: now.Add(-4 * 24 * time.Hour), duration: 2050 * time.Millisecond,
			terms: repairTerms,
		},
		{
			id: "run_c41b6e9d7f08", label: "Cytokine panel — 刺激实验复现",
			status: model.StatusFailed, organism: "hsapiens",
			genes:   []string{"IL1B", "IL6", "TNF", "CXCL8", "CCL2", "NFKB1", "RELA", "STAT3", "JUN", "FOS", "MAPK1", "MAPK3"},
			sources: []string{"GO:BP", "KEGG"}, created: now.Add(-21 * time.Hour), duration: 1200 * time.Millisecond,
			errMsg: "上游返回 HTTP 429（请求过于频繁），重试 2 次后放弃。",
		},
		{
			id: "run_8e2d5b3a9c61", label: "Neuron differentiation markers",
			status: model.StatusCanceled, organism: "mmusculus",
			genes:   []string{"NEFL", "NEFM", "TUBB3", "MAP2", "SOX2", "NES", "DCX", "STMN2"},
			sources: []string{"GO:BP"}, created: now.Add(-5 * 24 * time.Hour), duration: 640 * time.Millisecond,
		},
		{
			id: "run_1b7f0d2e5c93", label: "Mitochondrial import genes — 中断",
			status: model.StatusInterrupted, organism: "scevisiae",
			genes:   []string{"TOM20", "TOM22", "TOM70", "TIM23", "TIM17", "SSC1", "MIA40", "ERV1"},
			sources: []string{"GO:BP", "KEGG"}, created: now.Add(-7 * 24 * time.Hour), duration: 890 * time.Millisecond,
			errMsg: "服务重启，运行中的任务被标记为中断。",
		},
		{
			id: "run_3c9a80b4e7d2", label: "Autophagy core machinery",
			status: model.StatusQueued, organism: "hsapiens",
			genes:   []string{"ATG3", "ATG4B", "ATG5", "ATG7", "ATG9A", "BECN1", "MAP1LC3B", "SQSTM1", "ULK1", "WIPI2"},
			sources: []string{"GO:BP", "KEGG"}, created: now.Add(-3 * time.Minute), duration: 0,
		},
	}

	// Extra rows spread over the last two weeks so the dashboard chart and the
	// history table look like a system that has been used for a while. Each row
	// reuses one of the curated scenarios, because a "succeeded" run without
	// stored terms would be a misleading state.
	type scenario struct {
		name  string
		genes []string
		terms []termSeed
	}
	scenarios := []scenario{
		{"细胞周期基因集", cellCycleGenes, cellCycleTerms},
		{"凋亡通路 panel", apoptosisGenes, apoptosisTerms},
		{"免疫活化 signature", immuneGenes, immuneTerms},
		{"DNA 修复基因集", repairGenes, repairTerms},
	}

	perDay := []int{1, 0, 2, 1, 0, 3, 1, 2, 4, 2, 5, 3, 6, 4}
	labelIdx := 0
	dayIdx := 0
	for offset := len(perDay) - 1; offset >= 0; offset-- {
		count := perDay[offset]
		for i := 0; i < count; i++ {
			hour := 9 + (i*3+dayIdx*2)%9
			minute := (i*17 + dayIdx*11) % 60
			created := now.AddDate(0, 0, -offset)
			created = time.Date(created.Year(), created.Month(), created.Day(), hour, minute, 0, 0, created.Location())
			if created.After(now) {
				created = now.Add(-time.Duration(30+i) * time.Minute)
			}

			status := model.StatusSucceeded
			errMsg := ""
			if (offset+i)%11 == 3 {
				status = model.StatusFailed
				errMsg = "上游返回 HTTP 500，已记录失败原因。"
			}

			sc := scenarios[labelIdx%len(scenarios)]
			fillerTerms := sc.terms
			if status != model.StatusSucceeded {
				fillerTerms = nil
			}

			specs = append(specs, demoRun{
				id:       fmt.Sprintf("run_%02x%04x%04x", offset, i+1, 0xa5+dayIdx),
				label:    fmt.Sprintf("%s · %s", sc.name, fillerBatches[(labelIdx/len(scenarios))%len(fillerBatches)]),
				status:   status,
				organism: "hsapiens",
				genes:    sc.genes,
				sources:  allSources,
				created:  created,
				duration: time.Duration(700+((labelIdx*137)%1800)) * time.Millisecond,
				errMsg:   errMsg,
				terms:    fillerTerms,
			})
			labelIdx++
			dayIdx++
		}
	}

	sort.Slice(specs, func(i, j int) bool { return specs[i].created.After(specs[j].created) })

	runs := make([]model.Run, 0, len(specs))
	terms := make([]model.Term, 0, 64)
	events := make([]model.RunEvent, 0, len(specs)*3)

	for _, spec := range specs {
		symbols := gene.Normalize(spec.genes)
		hash := gene.Hash(symbols, spec.organism)

		run := model.Run{
			ID:           spec.id,
			Label:        spec.label,
			Status:       spec.status,
			Organism:     spec.organism,
			GeneCount:    len(symbols),
			Genes:        strings.Join(symbols, "\n"),
			InputHash:    hash,
			Params:       model.Params{Significance: "0.05", Correction: "g_SCS", Background: "annotated", Sources: spec.sources},
			ResultCount:  len(spec.terms),
			DurationMS:   spec.duration.Milliseconds(),
			DataVersion:  DemoDataVersion,
			IsDemo:       true,
			ErrorMessage: spec.errMsg,
			CreatedAt:    spec.created,
		}

		started := spec.created.Add(220 * time.Millisecond)
		run.StartedAt = &started
		if model.IsTerminal(spec.status) {
			finished := started.Add(spec.duration)
			run.FinishedAt = &finished
		}
		if spec.status == model.StatusQueued {
			run.StartedAt = nil
			run.DurationMS = 0
			run.DataVersion = ""
		}

		events = append(events, model.RunEvent{
			RunID: spec.id, Kind: "submitted", At: spec.created,
			Message: fmt.Sprintf("接收 %d 个基因，物种 %s", len(symbols), spec.organism),
		})
		if spec.status != model.StatusQueued {
			events = append(events, model.RunEvent{
				RunID: spec.id, Kind: "provider.request", At: started,
				Message: "POST g:Profiler /api/gost/profile（演示记录）",
			})
		}
		switch spec.status {
		case model.StatusSucceeded:
			events = append(events, model.RunEvent{
				RunID: spec.id, Kind: "completed", At: *run.FinishedAt,
				Message: fmt.Sprintf("返回 %d 条显著条目，数据集版本 %s", len(spec.terms), DemoDataVersion),
			})
		case model.StatusFailed:
			events = append(events, model.RunEvent{
				RunID: spec.id, Kind: "failed", At: *run.FinishedAt, Message: spec.errMsg,
			})
		case model.StatusCanceled:
			events = append(events, model.RunEvent{
				RunID: spec.id, Kind: "canceled", At: *run.FinishedAt, Message: "用户取消了该任务。",
			})
		case model.StatusInterrupted:
			events = append(events, model.RunEvent{
				RunID: spec.id, Kind: "interrupted", At: *run.FinishedAt, Message: spec.errMsg,
			})
		}

		for i, t := range spec.terms {
			terms = append(terms, model.Term{
				RunID:        spec.id,
				Source:       t.source,
				TermID:       t.native,
				Name:         t.name,
				PValue:       t.p,
				SourceSize:   t.size,
				QuerySize:    len(symbols),
				Intersection: len(t.genes),
				Genes:        strings.Join(t.genes, ","),
				Rank:         i + 1,
			})
		}

		runs = append(runs, run)
	}

	return runs, terms, events
}
