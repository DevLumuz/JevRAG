package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"sort"
	"unicode"

	"jev/internal/datasets"
	"jev/internal/embeddings"
	"jev/internal/evaluator"
	"jev/internal/extract/legal"
	"jev/internal/graphmodel"
)

const (
	embedModel = "gemini-embedding-001"
	// Vectors are always stored at full size. Price is per input token, so
	// 3,072 costs the same as 768; smaller sizes are derived locally with
	// embeddings.Reduce (--dims) without paying again.
	storeDims = 3072
	// US$ per million input tokens for gemini-embedding-001 (standard API).
	embedPricePerMTok = 0.15

	// gemini-embedding-001 reads at most 2,048 tokens per text and drops the
	// rest. Measured on KoBLEX: ~4.35 chars/token, so 8,500 chars stays just
	// under the limit. Cutting client-side loses nothing the model would have
	// seen and avoids paying for text it discards (the corpus has articles of
	// up to 5.2M chars).
	maxEmbedChars = 8500
)

// Query variants: which task type embeds the question, and which text.
var queryTasks = map[string]string{
	"retrieval_query":    embeddings.TaskRetrievalQuery,
	"question_answering": embeddings.TaskQuestionAnswering,
}

// embedSpace bundles a Gemini client with the store for its embedding space.
type embedSpace struct {
	client *embeddings.GeminiClient
	store  *embeddings.Store
}

func openSpace(ctx context.Context, cfg config, taskType string) (*embedSpace, error) {
	client, err := embeddings.NewGeminiClient(ctx, embeddings.GeminiOptions{TaskType: taskType, Dimensions: storeDims})
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(cfg.cacheDir, "embeddings", embedModel, fmt.Sprintf("%s-%d", taskType, storeDims))
	store, err := embeddings.OpenStore(dir, embeddings.StoreMeta{Model: embedModel, TaskType: taskType, Dims: storeDims})
	if err != nil {
		return nil, err
	}
	return &embedSpace{client: client, store: store}, nil
}

// docText is the exact text embedded for a statute node.
func docText(n *graphmodel.Node) string {
	return truncateRunes(legal.EmbeddingText(n), maxEmbedChars)
}

// queryText is what the judge reads for a question: the scenario
// (background) plus the question itself, both in English. Never the answer.
func queryText(q *datasets.KoBLEXRow) string {
	if q.BackgroundEng == "" {
		return q.QuestionEng
	}
	return q.BackgroundEng + "\n\n" + q.QuestionEng
}

// queryEmbedText is the text embedded for a question under --query-text.
func queryEmbedText(q *datasets.KoBLEXRow, variant string) string {
	if variant == "question" {
		return q.QuestionEng
	}
	return queryText(q)
}

// embedJob is one set of texts to embed in one space.
type embedJob struct {
	name  string
	space *embedSpace
	texts []string
}

// embedAll prints the estimated cost of every job, refuses to spend without
// --confirm-embed, then embeds within the --max-embed-tokens cap.
func embedAll(ctx context.Context, cfg config, jobs []embedJob) ([][][]float64, error) {
	opts := embeddings.RunOptions{
		Confirm:   cfg.confirmEmbed,
		MaxTokens: cfg.maxEmbedTokens,
		Log:       log.Printf,
	}

	var total embeddings.Estimate
	for _, j := range jobs {
		e := embeddings.EstimateCost(j.space.store, j.texts, opts)
		log.Printf("embeddings %-10s %6d texts · %6d to embed · %4d requests · ~%.2fM tokens · ~US$%.3f",
			j.name, e.Texts, e.Missing, e.Requests, float64(e.Tokens)/1e6, e.USD(embedPricePerMTok))
		total.Missing += e.Missing
		total.Tokens += e.Tokens
	}
	if total.Missing > 0 {
		log.Printf("embeddings total: ~%.2fM tokens · ~US$%.3f (estimate at 4 chars/token; cap --max-embed-tokens=%d)",
			float64(total.Tokens)/1e6, total.USD(embedPricePerMTok), cfg.maxEmbedTokens)
		if !cfg.confirmEmbed {
			return nil, fmt.Errorf("%d texts need paid embedding; re-run with --confirm-embed to proceed", total.Missing)
		}
	}

	out := make([][][]float64, len(jobs))
	for i, j := range jobs {
		vecs, err := embeddings.EmbedAll(ctx, j.space.client, j.space.store, j.texts, opts)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", j.name, err)
		}
		out[i] = vecs
	}
	return out, nil
}

// reduceAll derives --dims vectors from stored full-size ones and checks they
// all have the expected length (cosine silently returns 0 on a mismatch).
func reduceAll(vecs [][]float64, dims int) ([][]float64, error) {
	out := make([][]float64, len(vecs))
	for i, v := range vecs {
		if len(v) != storeDims {
			return nil, fmt.Errorf("vector %d has %d dims, want %d", i, len(v), storeDims)
		}
		out[i] = embeddings.Reduce(v, dims)
	}
	return out, nil
}

// truncateRunes returns s cut to at most n runes, without splitting a rune.
func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// --- Smoke test ---

// runSmoke checks the live embedding API before a full paid run (~US$0.03,
// of which the densest real batch is kept as real progress):
//
//	(a) same text twice → identical vectors; different texts → different
//	(b) [X, Y] then [Y, X] → vectors swap (order is preserved)
//	(c) the 100 densest corpus texts go through the normal store path
//	(d) one of them embedded alone matches its in-batch vector
//	(e) dims and norms are reported
func runSmoke(ctx context.Context, cfg config, nodes []*graphmodel.Node) error {
	if !cfg.confirmEmbed {
		return errors.New("--smoke calls the paid API (~US$0.03); add --confirm-embed")
	}
	docs, err := openSpace(ctx, cfg, embeddings.TaskRetrievalDocument)
	if err != nil {
		return err
	}

	a := "Commercial Act — Article 814 (Termination of Claims and Obligations of Carriers) The claims and obligations of a carrier shall be terminated where no judicial claim is made within one year."
	b := "Civil Act — Article 750 (Definition of Torts) Any person who causes losses to or inflicts injuries on another person by an unlawful act shall be bound to make compensation."

	v, err := docs.client.EmbedBatch(ctx, []string{a, a, b})
	if err != nil {
		return fmt.Errorf("(a): %w", err)
	}
	same, diff := evaluator.CosineSimilarity(v[0], v[1]), evaluator.CosineSimilarity(v[0], v[2])
	log.Printf("(a) dims %d · cos(A,A)=%.6f · cos(A,B)=%.4f", len(v[0]), same, diff)
	if same < 0.9999 || diff > 0.98 {
		return fmt.Errorf("(a) failed: cos(A,A)=%.6f, cos(A,B)=%.4f", same, diff)
	}

	xy, err := docs.client.EmbedBatch(ctx, []string{a, b})
	if err != nil {
		return fmt.Errorf("(b): %w", err)
	}
	yx, err := docs.client.EmbedBatch(ctx, []string{b, a})
	if err != nil {
		return fmt.Errorf("(b): %w", err)
	}
	o1, o2 := evaluator.CosineSimilarity(xy[0], yx[1]), evaluator.CosineSimilarity(xy[1], yx[0])
	log.Printf("(b) order kept: cos=%.6f, %.6f", o1, o2)
	if o1 < 0.9999 || o2 < 0.9999 {
		return errors.New("(b) failed: batch order not preserved")
	}

	dense := densestTexts(nodes, 100)
	log.Printf("(c) densest batch: %d texts, ~%d estimated tokens (digits counted as 1 token each)", len(dense), estimateDense(dense))
	vecs, err := embedAll(ctx, cfg, []embedJob{{name: "smoke", space: docs, texts: dense}})
	if err != nil {
		return fmt.Errorf("(c): %w", err)
	}

	alone, err := docs.client.EmbedBatch(ctx, []string{dense[0]})
	if err != nil {
		return fmt.Errorf("(d): %w", err)
	}
	cd := evaluator.CosineSimilarity(alone[0], vecs[0][0])
	log.Printf("(d) alone vs in-batch: cos=%.6f", cd)
	if cd < 0.999 {
		return fmt.Errorf("(d) failed: cos=%.6f", cd)
	}

	minN, maxN := math.Inf(1), 0.0
	for _, v := range vecs[0] {
		n := 0.0
		for _, x := range v {
			n += x * x
		}
		n = math.Sqrt(n)
		minN, maxN = math.Min(minN, n), math.Max(maxN, n)
	}
	log.Printf("(e) norms in [%.4f, %.4f] · store now holds %d vectors", minN, maxN, docs.store.Len())
	log.Printf("smoke test passed")
	return nil
}

// densestTexts returns the n distinct corpus texts with the most estimated
// tokens, counting digits as a full token (tables of numbers tokenize badly).
func densestTexts(nodes []*graphmodel.Node, n int) []string {
	seen := map[string]bool{}
	var texts []string
	for _, nd := range nodes {
		t := docText(nd)
		if !seen[t] {
			seen[t] = true
			texts = append(texts, t)
		}
	}
	sort.SliceStable(texts, func(i, j int) bool { return denseScore(texts[i]) > denseScore(texts[j]) })
	return texts[:min(n, len(texts))]
}

func denseScore(s string) float64 {
	digits, other := 0, 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			digits++
		} else {
			other++
		}
	}
	return float64(digits) + float64(other)/4.35
}

func estimateDense(texts []string) int {
	total := 0.0
	for _, t := range texts {
		total += denseScore(t)
	}
	return int(total)
}
