package pipeline

import "testing"

func TestNormalizeEmbeddingEndpoint(t *testing.T) {
	t.Parallel()

	if got := normalizeEmbeddingEndpoint("http://127.0.0.1:8844"); got != "http://127.0.0.1:8844/embed" {
		t.Fatalf("unexpected endpoint normalization: %q", got)
	}
	if got := normalizeEmbeddingEndpoint("http://127.0.0.1:8844/v1/embeddings"); got != "http://127.0.0.1:8844/v1/embeddings" {
		t.Fatalf("unexpected endpoint normalization for explicit path: %q", got)
	}
}

func TestToVectorLiteralDimensionValidation(t *testing.T) {
	t.Parallel()

	_, err := toVectorLiteral([]float64{0.1, 0.2})
	if err == nil {
		t.Fatalf("expected dimension validation error for short vector")
	}
}

func TestEmbeddingRequestShapeAndResponseVectors(t *testing.T) {
	t.Parallel()

	standard := newEmbedRequest(EmbedOptions{Endpoint: "http://127.0.0.1:8844/embed", MaxLength: 128}, []string{"a"})
	if len(standard.Texts) != 1 || standard.MaxLength != 128 || len(standard.Input) != 0 {
		t.Fatalf("standard embed request = %#v, want texts payload", standard)
	}
	openAI := newEmbedRequest(EmbedOptions{Endpoint: "http://127.0.0.1:8844/v1/embeddings", MaxLength: 128}, []string{"a"})
	if len(openAI.Input) != 1 || len(openAI.Texts) != 0 || openAI.MaxLength != 0 {
		t.Fatalf("OpenAI embed request = %#v, want input payload", openAI)
	}

	elapsed := 12.5
	vectors, gotElapsed, err := vectorsFromEmbeddingResponse(embedResponse{
		ElapsedMS: &elapsed,
		Data: []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		}{
			{Index: 1, Embedding: []float64{0.2}},
			{Index: 0, Embedding: []float64{0.1}},
		},
	})
	if err != nil {
		t.Fatalf("vectorsFromEmbeddingResponse() error = %v", err)
	}
	if gotElapsed == nil || *gotElapsed != elapsed {
		t.Fatalf("elapsed = %v, want %v", gotElapsed, elapsed)
	}
	if len(vectors) != 2 || vectors[0][0] != 0.1 || vectors[1][0] != 0.2 {
		t.Fatalf("vectors = %#v, want sorted OpenAI data", vectors)
	}

	if _, _, err := vectorsFromEmbeddingResponse(embedResponse{}); err == nil {
		t.Fatalf("vectorsFromEmbeddingResponse(empty) error = nil, want missing vectors error")
	}
}

func TestSemanticThresholdHelpers(t *testing.T) {
	t.Parallel()

	if !shouldAutoMergeSemantic(0.97, 0.05) {
		t.Fatalf("expected override cosine threshold to auto-merge")
	}
	if !shouldAutoMergeSemantic(0.94, 0.35) {
		t.Fatalf("expected cosine+title threshold to auto-merge")
	}
	if shouldAutoMergeSemantic(0.92, 0.50) {
		t.Fatalf("did not expect low cosine to auto-merge")
	}

	composite := semanticCompositeScore(0.9, 0.4, 1.0)
	if composite <= 0 || composite > 1 {
		t.Fatalf("expected composite score in (0,1], got %f", composite)
	}
}
