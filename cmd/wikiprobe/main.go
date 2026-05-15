package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eko/iris-bot/internal/embedder"
	"github.com/eko/iris-bot/internal/repository"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: wikiprobe <query>")
		os.Exit(2)
	}
	query := os.Args[1]

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "db: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	emb, err := embedder.NewONNX(embedder.ONNXConfig{
		ModelPath:     os.Getenv("IRIS_EMBED_MODEL_PATH"),
		TokenizerPath: os.Getenv("IRIS_EMBED_TOKENIZER_PATH"),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "onnx: %v\n", err)
		os.Exit(1)
	}
	defer emb.Close()

	vec, err := emb.Embed(ctx, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "embed: %v\n", err)
		os.Exit(1)
	}
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	fmt.Printf("query: %q\nvec.dim=%d  vec[0..3]=%v  norm^2=%.4f\n\n", query, len(vec), vec[:3], norm)

	repo := repository.NewWikiRepo(repository.NewDB(pool))
	results, err := repo.SearchSimilar(ctx, "fandom_wutheringwaves", vec, 10)
	if err != nil {
		fmt.Fprintf(os.Stderr, "search: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("hits=%d\n", len(results))
	for i, r := range results {
		fmt.Printf("  [%d] dist=%.4f  score=%.4f  page=%d  title=%q\n", i, r.Distance, 1-r.Distance, r.PageID, r.Title)
	}
}
