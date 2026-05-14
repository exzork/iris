package canoncheck

import (
	"context"

	ragpkg "github.com/eko/iris-bot/internal/lore/rag"
)

const (
	ReasonSupported        = "Klaim didukung oleh sumber yang terindeks."
	ReasonContradicted     = "Sumber yang terindeks menunjukkan klaim ini tidak sesuai."
	ReasonUnsupported      = "Belum ada sumber yang terindeks mendukung klaim ini."
	ReasonNeedsMoreSources = "Sumber terindeks belum cukup untuk memverifikasi klaim."
)

type Verifier struct {
	Retriever      *ragpkg.Retriever
	TopK           int
	MinSupportScore float64
	MinChunks      int
}

func NewVerifier(retriever *ragpkg.Retriever) *Verifier {
	return &Verifier{
		Retriever:       retriever,
		TopK:            5,
		MinSupportScore: 0.6,
		MinChunks:       2,
	}
}

func (v *Verifier) Check(ctx context.Context, claim Claim) (*Verdict, error) {
	if claim.Text == "" {
		return &Verdict{
			Status:     StatusUnsupported,
			Confidence: 0.0,
			Reason:     "klaim kosong",
		}, nil
	}

	query := claim.Query
	if query == "" {
		query = claim.Text
	}

	chunks, err := v.Retriever.Retrieve(ctx, query, v.TopK)
	if err != nil {
		return nil, err
	}

	status := ClassifyContent(chunks, claim, v.MinChunks, v.MinSupportScore)

	verdict := &Verdict{
		Status: status,
		Reason: reasonForStatus(status),
	}

	citationMap := make(map[string]ragpkg.Citation)
	var snippets []string

	for _, chunk := range chunks {
		if chunk.Title != "" {
			citation := ragpkg.Citation{Title: chunk.Title, URL: chunk.URL}
			citationMap[chunk.Title] = citation
		}
		snippets = append(snippets, chunk.Content)
	}

	for _, citation := range citationMap {
		verdict.Citations = append(verdict.Citations, citation)
	}
	verdict.Snippets = snippets

	// Calculate confidence
	verdict.Confidence = calculateConfidence(status, chunks, v.MinSupportScore, v.MinChunks)

	return verdict, nil
}

func reasonForStatus(status Status) string {
	switch status {
	case StatusSupported:
		return ReasonSupported
	case StatusContradicted:
		return ReasonContradicted
	case StatusUnsupported:
		return ReasonUnsupported
	case StatusNeedsMoreSources:
		return ReasonNeedsMoreSources
	default:
		return ""
	}
}

func calculateConfidence(status Status, chunks []ragpkg.ScoredChunk, minSupportScore float64, minChunks int) float64 {
	switch status {
	case StatusSupported:
		if len(chunks) == 0 {
			return 0.0
		}
		// Average top 3 scores
		sum := 0.0
		count := 0
		for i, chunk := range chunks {
			if i >= 3 {
				break
			}
			sum += chunk.Score
			count++
		}
		if count == 0 {
			return 0.0
		}
		return sum / float64(count)

	case StatusContradicted:
		if len(chunks) == 0 {
			return 0.0
		}
		// 1 - average score
		sum := 0.0
		for _, chunk := range chunks {
			sum += chunk.Score
		}
		avg := sum / float64(len(chunks))
		return 1.0 - avg

	case StatusUnsupported:
		return 0.0

	case StatusNeedsMoreSources:
		if minChunks == 0 {
			return 0.0
		}
		strongChunks := 0
		for _, chunk := range chunks {
			if chunk.Score >= minSupportScore {
				strongChunks++
			}
		}
		return float64(strongChunks) / float64(minChunks)

	default:
		return 0.0
	}
}
