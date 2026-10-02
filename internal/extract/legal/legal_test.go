package legal_test

import (
	"testing"

	"jev/internal/datasets"
	"jev/internal/extract/legal"
)

func TestExtractCitations(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string // expected article numbers
	}{
		{
			"simple Article N",
			"as prescribed in Article 3 and Article 22",
			[]string{"3", "22"},
		},
		{
			"Article N (P) with paragraph",
			"under Article 67 (1) the operator shall",
			[]string{"67"},
		},
		{
			"subparagraph of Article",
			"subparagraph 8 of Article 4",
			[]string{"4"},
		},
		{
			"Articles with hyphen range",
			"Articles 10 through 15 shall apply",
			[]string{"10"},
		},
		{
			"no duplicates",
			"Article 3 blah Article 3 again",
			[]string{"3"},
		},
		{
			"self-title Article is not a citation",
			"Article 5 (Succession) rights under this Act",
			[]string{"5"},
		},
		{
			"no articles",
			"This section has no cross references at all",
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := legal.ExtractCitations(tt.content)
			if len(got) != len(tt.want) {
				t.Fatalf("ExtractCitations() = %v (len %d), want %v (len %d)",
					got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("citation[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestStatutesToNodes(t *testing.T) {
	statutes := []*datasets.StatuteRow{
		{
			IndexEng:   "CIVIL ACT / Article. 40 / Capacity",
			ContentEng: "Article 40 (Capacity) A person has capacity...",
		},
		{
			IndexEng:   "CIVIL ACT / Article. 41 / Restrictions",
			ContentEng: "Article 41 (Restrictions) As prescribed in Article 40, restrictions apply.",
		},
	}

	nodes := legal.StatutesToNodes(statutes)

	if len(nodes) != 2 {
		t.Fatalf("len(nodes) = %d, want 2", len(nodes))
	}

	// Check first node.
	n := nodes[0]
	if n.Key != "CIVIL ACT / Article. 40 / Capacity" {
		t.Errorf("Key = %q", n.Key)
	}
	if n.Label != "LegalArticle" {
		t.Errorf("Label = %q", n.Label)
	}
	if n.Content != "Article 40 (Capacity) A person has capacity..." {
		t.Errorf("Content = %q", n.Content)
	}
	if n.NumericID != 0 {
		t.Errorf("NumericID = %d, want 0", n.NumericID)
	}
	if nodes[1].NumericID != 1 {
		t.Errorf("second node NumericID = %d, want 1", nodes[1].NumericID)
	}
}

func TestBuildEdges(t *testing.T) {
	statutes := []*datasets.StatuteRow{
		{
			IndexEng:   "CIVIL ACT / Article. 40 / Capacity",
			ContentEng: "Article 40 (Capacity) A person has capacity.",
		},
		{
			IndexEng:   "CIVIL ACT / Article. 41 / Restrictions",
			ContentEng: "Article 41 (Restrictions) See Article 40 for base rules.",
		},
		{
			IndexEng:   "CRIMINAL ACT / Article. 330 / Larceny",
			ContentEng: "Article 330 (Larceny) Punishment as prescribed.",
		},
	}

	nodes := legal.StatutesToNodes(statutes)
	edges := legal.BuildEdges(nodes, statutes)

	// Article 41 references Article 40 within the same act (CIVIL ACT).
	// Article 40's self-reference should NOT produce an edge.
	// Article 330 has no cross-references.
	if len(edges) != 1 {
		t.Fatalf("len(edges) = %d, want 1", len(edges))
	}

	e := edges[0]
	if e.F.Key != "CIVIL ACT / Article. 41 / Restrictions" {
		t.Errorf("edge from = %q", e.F.Key)
	}
	if e.T.Key != "CIVIL ACT / Article. 40 / Capacity" {
		t.Errorf("edge to = %q", e.T.Key)
	}
	if e.Type != "references" {
		t.Errorf("edge type = %q", e.Type)
	}
	if e.Origin != "extracted" {
		t.Errorf("edge origin = %q", e.Origin)
	}
}

func TestBuildEdges_CrossActReference(t *testing.T) {
	// References to articles in OTHER acts should NOT match — we can only
	// resolve intra-act references reliably via the article number.
	statutes := []*datasets.StatuteRow{
		{
			IndexEng:   "CIVIL ACT / Article. 5 / Rights",
			ContentEng: "Article 5 (Rights) As per Article 330.",
		},
		{
			IndexEng:   "CRIMINAL ACT / Article. 330 / Larceny",
			ContentEng: "Article 330 (Larceny) ...",
		},
	}

	nodes := legal.StatutesToNodes(statutes)
	edges := legal.BuildEdges(nodes, statutes)

	// Article 5 (CIVIL ACT) cites "Article 330" but there's no Article 330
	// in the CIVIL ACT — only in CRIMINAL ACT. This cross-act reference
	// cannot be resolved reliably, so we expect 0 edges.
	if len(edges) != 0 {
		t.Fatalf("len(edges) = %d, want 0 (cross-act should not resolve)", len(edges))
	}
}

func TestExtractActName(t *testing.T) {
	tests := []struct {
		indexEng string
		want     string
	}{
		{"CIVIL ACT / Article. 40 / Capacity", "CIVIL ACT"},
		{"COMMERCIAL ACT / Article. 665 / Liability", "COMMERCIAL ACT"},
		{"ACT ON ACQUISITION / Article. 1 / Purpose", "ACT ON ACQUISITION"},
		{"STANDALONE", "STANDALONE"},
	}
	for _, tt := range tests {
		t.Run(tt.indexEng, func(t *testing.T) {
			got := legal.ExtractActName(tt.indexEng)
			if got != tt.want {
				t.Errorf("ExtractActName(%q) = %q, want %q", tt.indexEng, got, tt.want)
			}
		})
	}
}

func TestExtractArticleNumber(t *testing.T) {
	tests := []struct {
		indexEng string
		want     string
	}{
		{"CIVIL ACT / Article. 40 / Capacity", "40"},
		{"COMMERCIAL ACT / Article. 665 / Liability", "665"},
		{"COMMERCIAL ACT / Article. 4-2 / Limitation", "4-2"},
		{"NO ARTICLE HERE", ""},
	}
	for _, tt := range tests {
		t.Run(tt.indexEng, func(t *testing.T) {
			got := legal.ExtractArticleNumber(tt.indexEng)
			if got != tt.want {
				t.Errorf("ExtractArticleNumber(%q) = %q, want %q", tt.indexEng, got, tt.want)
			}
		})
	}
}

func TestCanonicalKey(t *testing.T) {
	tests := []struct {
		indexEng string
		want     string
	}{
		{"COMMERCIAL ACT / Article. 665 / Liability of Non-Life Insurers", "COMMERCIAL ACT#665"},
		{"COMMERCIAL ACT / Article. 665", "COMMERCIAL ACT#665"},
		{"CIVIL ACT / Article. 4-2 / Something", "CIVIL ACT#4-2"},
		{"PUBLIC INTEREST WHISTLEBLOWER PROTECTION ACT / Article.22 / Request", "PUBLIC INTEREST WHISTLEBLOWER PROTECTION ACT#22"},
		{"  ODD FORMAT WITHOUT ARTICLE  ", "ODD FORMAT WITHOUT ARTICLE"},
	}
	for _, tt := range tests {
		if got := legal.CanonicalKey(tt.indexEng); got != tt.want {
			t.Errorf("CanonicalKey(%q) = %q, want %q", tt.indexEng, got, tt.want)
		}
	}
}

func TestGoldKeys(t *testing.T) {
	row := &datasets.KoBLEXRow{Contexts: []datasets.KoBLEXProvision{
		{IndexEng: "COMMERCIAL ACT / Article. 665 / Liability"},
		{IndexEng: "COMMERCIAL ACT / Article. 665"}, // same article, other paragraph
		{IndexEng: "COMMERCIAL ACT / Article. 666"},
	}}
	got := legal.GoldKeys(row)
	want := []string{"COMMERCIAL ACT#665", "COMMERCIAL ACT#666"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("GoldKeys() = %v, want %v", got, want)
	}
}

func TestCanonicalRanking(t *testing.T) {
	got := legal.CanonicalRanking([]string{
		"CIVIL ACT / Article. 1 / A",
		"CIVIL ACT / Article. 1",
		"CIVIL ACT / Article. 2 / B",
	})
	want := []string{"CIVIL ACT#1", "CIVIL ACT#2"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("CanonicalRanking() = %v, want %v", got, want)
	}
}

func TestActDisplayName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"COMMERCIAL ACT", "Commercial Act"},
		{"ENFORCEMENT DECREE OF THE INCOME TAX ACT", "Enforcement Decree of the Income Tax Act"},
		{"ACT ON THE ESTABLISHMENT, OPERATION, ETC. OF TEACHERS’ UNIONS", "Act on the Establishment, Operation, Etc. of Teachers’ Unions"},
		{"Enforcement Decree of the Urban Railway Act", "Enforcement Decree of the Urban Railway Act"}, // already mixed case
	}
	for _, tt := range tests {
		if got := legal.ActDisplayName(tt.in); got != tt.want {
			t.Errorf("ActDisplayName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCleanForEmbedding(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"amendment tags", "(1) The insurer shall pay. <Amended on Mar. 2, 2020> (2) Next. <Newly Inserted on Dec. 29, 2021; Feb. 1, 2022>", "(1) The insurer shall pay. (2) Next."},
		{"bare date tag", "Article 5 Deleted. <Dec. 29, 2020>", "Article 5 Deleted."},
		{"image refs", "formula:<img id=\"40470045\"></img> applies", "formula: applies"},
		{"machine translation marker", "%MACHINE_TRANSLATED% Article 1 (Purpose) This Act", "Article 1 (Purpose) This Act"},
		{"keeps legal brackets without years", "<omitted> text", "<omitted> text"},
	}
	for _, tt := range tests {
		if got := legal.CleanForEmbedding(tt.in); got != tt.want {
			t.Errorf("%s: CleanForEmbedding() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestEmbeddingText(t *testing.T) {
	statutes := []*datasets.StatuteRow{{
		IndexEng:   "COMMERCIAL ACT / Article. 814 / Termination",
		ContentEng: "%MACHINE_TRANSLATED% Article 814 (Termination) Claims end. <Amended on Mar. 2, 2020>",
	}}
	n := legal.StatutesToNodes(statutes)[0]
	if got, want := legal.EmbeddingText(n), "Commercial Act — Article 814 (Termination) Claims end."; got != want {
		t.Errorf("EmbeddingText() = %q, want %q", got, want)
	}
	if n.Properties["machine_translated"] != "true" {
		t.Error("machine_translated property not set")
	}
	if n.Content != statutes[0].ContentEng {
		t.Error("node Content must keep the raw text")
	}
}
