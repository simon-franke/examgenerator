// Command examgen generates LaTeX exams from an existing exam and lecture slides.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/simon-franke/examgenerator/internal/config"
	"github.com/simon-franke/examgenerator/internal/discord"
	"github.com/simon-franke/examgenerator/internal/embedding"
	"github.com/simon-franke/examgenerator/internal/generate"
	"github.com/simon-franke/examgenerator/internal/ingest"
	"github.com/simon-franke/examgenerator/internal/latex"
	"github.com/simon-franke/examgenerator/internal/llm"
	"github.com/simon-franke/examgenerator/internal/pdf"
	"github.com/simon-franke/examgenerator/internal/rag"
	"github.com/simon-franke/examgenerator/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	if err := loadDotenv(".env"); err != nil {
		fmt.Fprintln(os.Stderr, "error: loading .env:", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	switch os.Args[1] {
	case "ingest":
		err = runIngest(ctx, os.Args[2:])
	case "generate":
		err = runGenerate(ctx, os.Args[2:])
	case "deliver":
		err = runDeliver(ctx, os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Generate a new exam from an old one + lecture slides

Commands:
  ingest    Embed lecture slides into the vector store (RAG corpus)
  generate  Analyze an old exam (as images) and produce a new LaTeX exam
  deliver   Send saved exam files to your configured private Discord DM

Run "examgen <command> -h" for command flags.
Configuration is via EXAMGEN_* environment variables (see .env.example).
`)
}

func runIngest(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("ingest", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	slidesValue := flags.String("slides", "", "comma-separated slide PDF paths")
	dirValue := flags.String("dir", "", "directory containing slide PDFs")
	sourceValue := flags.String("source", "", "logical source name (groups all supplied PDFs)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("ingest: unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	paths, err := slidePaths(*slidesValue, *dirValue)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	embedder, err := embedding.New(cfg.EmbeddingProvider, cfg.EmbeddingAPIKey, cfg.EmbeddingModel, cfg.EmbeddingBaseURL, cfg.EmbeddingDimensions)
	if err != nil {
		return fmt.Errorf("create embedder: %w", err)
	}
	vectorStore, err := store.NewPostgres(ctx, cfg.DatabaseURL, embedder.Dimensions())
	if err != nil {
		return err
	}
	defer vectorStore.Close()
	if err := vectorStore.Init(ctx); err != nil {
		return err
	}
	ingester := ingest.New(pdf.NewPdftotext(), embedder, vectorStore, cfg.ChunkSize, cfg.ChunkOverlap)
	if *sourceValue != "" {
		count, err := ingester.IngestMany(ctx, paths, *sourceValue)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "ingested %d chunks from %d PDF(s) as %q\n", count, len(paths), *sourceValue)
		return nil
	}
	for _, path := range paths {
		source := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		count, err := ingester.Ingest(ctx, path, source)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "ingested %d chunks from %s as %q\n", count, path, source)
	}
	return nil
}

func runGenerate(ctx context.Context, args []string) (resultErr error) {
	flags := flag.NewFlagSet("generate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	examPath := flags.String("exam", "", "old exam PDF path (required)")
	outPath := flags.String("out", "output/new-exam.tex", "output LaTeX path")
	dpi := flags.Int("dpi", 150, "PDF rendering resolution")
	debugPath := flags.String("debug", "", "optional model/tool trace file")
	discordEnabled := flags.Bool("discord", discordDefault(), "compile PDF and deliver to your configured Discord DM")
	includeSource := flags.Bool("include-source", false, "also attach LaTeX source to the Discord DM")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("generate: unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	if strings.TrimSpace(*examPath) == "" {
		return errors.New("generate: --exam is required")
	}
	if *dpi <= 0 {
		return fmt.Errorf("generate: --dpi must be positive, got %d", *dpi)
	}
	if err := validateRegularFile(*examPath, "exam PDF"); err != nil {
		return err
	}

	var delivery *discord.Client
	if *discordEnabled {
		if !strings.EqualFold(filepath.Ext(*outPath), ".tex") {
			return errors.New("Discord delivery requires a .tex output path")
		}
		var err error
		delivery, err = configuredDiscord()
		if err != nil {
			return err
		}
		if err := latex.CheckCompiler(ctx); err != nil {
			return err
		}
		defer func() {
			if resultErr != nil && ctx.Err() == nil {
				if err := delivery.Send(ctx, "Exam generation or delivery failed. Check the local CLI output for details; any saved files remain on your computer."); err != nil {
					fmt.Fprintf(os.Stderr, "could not send Discord failure notification: %v\n", err)
				}
			}
		}()
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.ValidateGeneration(); err != nil {
		return err
	}
	embedder, err := embedding.New(cfg.EmbeddingProvider, cfg.EmbeddingAPIKey, cfg.EmbeddingModel, cfg.EmbeddingBaseURL, cfg.EmbeddingDimensions)
	if err != nil {
		return fmt.Errorf("create embedder: %w", err)
	}
	vectorStore, err := store.NewPostgres(ctx, cfg.DatabaseURL, embedder.Dimensions())
	if err != nil {
		return err
	}
	defer vectorStore.Close()
	if err := vectorStore.EnsureSchemaAndNotEmpty(ctx); err != nil {
		return err
	}
	client, err := llm.New(cfg.Provider, cfg.APIKey, cfg.Model, cfg.BaseURL, cfg.RequestTimeout)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	fmt.Fprintf(os.Stderr, "rendering %s at %d DPI\n", *examPath, *dpi)
	pages, err := pdf.NewPdftoppm().Render(ctx, *examPath, *dpi)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "rendered %d exam page(s)\n", len(pages))
	handler := rag.NewHandler(vectorStore, embedder)
	generator := generate.New(client, handler, cfg.Model, cfg.MaxTokens, cfg.Temperature, cfg.MaxToolHops)
	generator.SetProgress(func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	})
	if *debugPath != "" {
		if err := os.MkdirAll(filepath.Dir(*debugPath), 0o755); err != nil {
			return fmt.Errorf("create debug directory: %w", err)
		}
		debugFile, err := os.Create(*debugPath)
		if err != nil {
			return fmt.Errorf("create debug trace: %w", err)
		}
		defer debugFile.Close()
		generator.SetDebug(debugFile)
	}
	raw, err := generator.Generate(ctx, pages)
	if err != nil {
		return err
	}
	document := latex.Clean(raw)
	if !strings.Contains(document, `\documentclass`) || !strings.Contains(document, `\end{document}`) {
		return errors.New("model response did not contain a complete LaTeX document")
	}
	if err := latex.Write(*outPath, document); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", *outPath)
	if delivery != nil {
		pdfPath, err := latex.CompilePDF(ctx, *outPath)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", pdfPath)
		paths := []string{pdfPath}
		if *includeSource {
			paths = append(paths, *outPath)
		}
		if err := delivery.Send(ctx, "Your practice exam is ready. Review the generated questions before use.", paths...); err != nil {
			return fmt.Errorf("exam saved locally; Discord delivery failed: %w (retry with examgen deliver --pdf %q)", err, pdfPath)
		}
		fmt.Fprintln(os.Stderr, "delivered exam to your private Discord DM")
	}
	return nil
}

func slidePaths(slidesValue, dirValue string) ([]string, error) {
	var paths []string
	for _, path := range strings.Split(slidesValue, ",") {
		if path = strings.TrimSpace(path); path != "" {
			paths = append(paths, path)
		}
	}
	if strings.TrimSpace(dirValue) != "" {
		entries, err := os.ReadDir(dirValue)
		if err != nil {
			return nil, fmt.Errorf("read slides directory: %w", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".pdf") {
				paths = append(paths, filepath.Join(dirValue, entry.Name()))
			}
		}
	}
	if len(paths) == 0 {
		return nil, errors.New("ingest: provide at least one PDF with --slides or --dir")
	}
	sort.Strings(paths)
	unique := paths[:0]
	for _, path := range paths {
		if len(unique) == 0 || unique[len(unique)-1] != path {
			unique = append(unique, path)
		}
	}
	for _, path := range unique {
		if !strings.EqualFold(filepath.Ext(path), ".pdf") {
			return nil, fmt.Errorf("slide file %q does not have a .pdf extension", path)
		}
		if err := validateRegularFile(path, "slide PDF"); err != nil {
			return nil, err
		}
	}
	return unique, nil
}

func validateRegularFile(path, description string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("open %s %q: %w", description, path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s %q is not a regular file", description, path)
	}
	return nil
}
