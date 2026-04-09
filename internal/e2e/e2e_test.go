package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sem/internal/app"
	"sem/internal/chunk"
	"sem/internal/config"
	"sem/internal/embed"
	"sem/internal/indexer"
	"sem/internal/log"
	"sem/internal/output"
	"sem/internal/storage"
)

// skipIfNoONNX skips the test if ONNX Runtime is not available.
// E2E tests require real embeddings to produce meaningful results.
func skipIfNoONNX(t *testing.T) {
	t.Helper()
	libPath := os.Getenv("ONNXRUNTIME_SO_PATH")
	if libPath == "" {
		commonPaths := []string{
			"/opt/homebrew/lib/libonnxruntime.dylib",
			"/usr/local/lib/libonnxruntime.dylib",
			"/usr/local/lib/libonnxruntime.so",
		}
		found := false
		for _, p := range commonPaths {
			if _, err := os.Stat(p); err == nil {
				found = true
				break
			}
		}
		if !found {
			t.Skip("ONNX Runtime not available — skipping E2E test")
		}
	}
}

// sharedModelDir is a model cache shared across all E2E tests so the model
// downloads only once instead of once per parallel test.
var sharedModelDir string

func init() {
	home, err := os.UserHomeDir()
	if err == nil {
		sharedModelDir = filepath.Join(home, ".sem", "models")
	} else {
		sharedModelDir = os.TempDir()
	}
	_ = os.MkdirAll(sharedModelDir, 0o755)
}

// testEnv holds all the pieces needed for an E2E test.
type testEnv struct {
	t         *testing.T
	tmpDir    string
	paths     app.Paths
	cfg       config.Config
	sourceDir string
	logger    *log.Logger
}

// setupTestEnv creates a temporary directory, configures a source, and returns the env.
func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("create source dir: %v", err)
	}

	baseDir := filepath.Join(tmpDir, ".sem")
	bundlesDir := filepath.Join(baseDir, "bundles")
	backendsDir := filepath.Join(baseDir, "backends")

	paths := app.Paths{
		HomeDir:     tmpDir,
		BaseDir:     baseDir,
		ConfigPath:  filepath.Join(baseDir, "config.toml"),
		BundlesDir:  bundlesDir,
		BackendsDir: backendsDir,
		LanceDBDir:  filepath.Join(backendsDir, "lancedb"),
		ModelsDir:   sharedModelDir,
	}

	for _, dir := range []string{baseDir, bundlesDir, backendsDir, paths.LanceDBDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create dir %s: %v", dir, err)
		}
	}

	cfg := config.Config{
		General: config.GeneralConfig{
			DefaultBundle: "default",
		},
		Embedding: config.EmbeddingConfig{
			Mode:          "light",
			ModelCacheDir: sharedModelDir,
			BatchSize:     32,
			MaxTokens:     256,
			Normalize:     true,
		},
		Storage: config.StorageConfig{
			BundleDir: bundlesDir,
			Backend:   "lancedb",
			LanceDB: config.LanceDBStorageConfig{
				Path:   paths.LanceDBDir,
				Table:  "chunks",
				Metric: "cosine",
			},
		},
		Chunking: config.ChunkingConfig{
			MaxChars:        2200,
			OverlapChars:    300,
			MinChars:        400,
			RespectHeadings: true,
		},
		Ignore: config.IgnoreConfig{
			DefaultPatterns: []string{".git", "node_modules"},
			UseGitignore:    true,
		},
		Sources: []config.SourceConfig{
			{
				Name:              "test-source",
				Path:              sourceDir,
				Enabled:           true,
				IncludeExtensions: []string{"md", "go", "py", "yaml", "yml", "json", "txt", "toml", "rs", "ts", "js"},
			},
		},
	}

	return &testEnv{
		t:         t,
		tmpDir:    tmpDir,
		paths:     paths,
		cfg:       cfg,
		sourceDir: sourceDir,
		logger:    log.New(false),
	}
}

// writeTestFile writes content to a file within the test source directory.
func (e *testEnv) writeTestFile(relPath, content string) {
	e.t.Helper()
	path := filepath.Join(e.sourceDir, relPath)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		e.t.Fatalf("create dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		e.t.Fatalf("write file %s: %v", path, err)
	}
}

// runIndex runs the indexer and returns the result.
func (e *testEnv) runIndex(full bool) indexer.SyncResult {
	e.t.Helper()
	ctx := context.Background()
	result, err := indexer.Run(ctx, e.paths, e.cfg, "", full, e.logger, nil)
	if err != nil {
		e.t.Fatalf("index: %v", err)
	}
	return result
}

// runSync runs incremental sync.
func (e *testEnv) runSync() indexer.SyncResult {
	e.t.Helper()
	ctx := context.Background()
	result, err := indexer.Run(ctx, e.paths, e.cfg, "", false, e.logger, nil)
	if err != nil {
		e.t.Fatalf("sync: %v", err)
	}
	return result
}

// runSearch performs a semantic search and returns results.
func (e *testEnv) runSearch(query string, limit int) []output.SearchResult {
	e.t.Helper()

	bundle := storage.NewBundle(e.paths.BundleDir(e.cfg.General.DefaultBundle))
	model, err := bundle.LoadModel()
	if err != nil {
		e.t.Fatalf("load model: %v", err)
	}

	service, err := embed.NewServiceWithModelDir(model.Mode, e.cfg.Embedding.ModelCacheDir)
	if err != nil {
		e.t.Fatalf("create embed service: %v", err)
	}
	defer service.Close()

	queryVector, err := service.EmbedQuery(context.Background(), query)
	if err != nil {
		e.t.Fatalf("embed query: %v", err)
	}

	store := storage.NewStore(e.paths.LanceDBDir)
	hits, err := store.Search(context.Background(), queryVector, limit*5)
	if err != nil {
		e.t.Fatalf("search: %v", err)
	}

	chunks, err := bundle.LoadChunks()
	if err != nil {
		e.t.Fatalf("load chunks: %v", err)
	}
	chunkMap := make(map[string]chunk.Record, len(chunks))
	for _, r := range chunks {
		chunkMap[r.ID] = r
	}

	results := make([]output.SearchResult, 0, len(hits))
	for _, hit := range hits {
		record, ok := chunkMap[hit.ChunkID]
		if !ok {
			continue
		}
		results = append(results, output.SearchResult{
			ChunkID:    record.ID,
			FilePath:   record.FilePath,
			Snippet:    record.Content,
			Score:      hit.Score,
			SourceName: record.SourceName,
			Metadata: output.ResultMetadata{
				FileKind:     record.Kind,
				Language:     record.Language,
				Title:        record.Title,
				StartLine:    record.StartLine,
				EndLine:      record.EndLine,
				FunctionName: record.FunctionName,
				SectionLevel: record.SectionLevel,
			},
		})
		if len(results) >= limit {
			break
		}
	}

	return results
}

// runSearchWithFilters performs a semantic search with post-search filters.
func (e *testEnv) runSearchWithFilters(query string, limit int, language, kind, dir string) []output.SearchResult {
	e.t.Helper()
	results := e.runSearch(query, limit*5)

	if language != "" {
		allowed := strings.Split(language, ",")
		for i := range allowed {
			allowed[i] = strings.TrimSpace(allowed[i])
		}
		var filtered []output.SearchResult
		for _, r := range results {
			for _, lang := range allowed {
				if strings.EqualFold(r.Metadata.Language, lang) {
					filtered = append(filtered, r)
					break
				}
			}
		}
		results = filtered
	}

	if kind != "" {
		var filtered []output.SearchResult
		for _, r := range results {
			if strings.EqualFold(r.Metadata.FileKind, kind) {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	if dir != "" {
		dir = strings.TrimRight(dir, "/") + "/"
		var filtered []output.SearchResult
		for _, r := range results {
			if strings.Contains(r.FilePath, dir) {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

// TestFullWorkflow tests the complete index → search workflow.
func TestFullWorkflow(t *testing.T) {
	skipIfNoONNX(t)

	env := setupTestEnv(t)

	env.writeTestFile("notes.md", `# Authentication Guide

Authentication is the process of verifying the identity of a user or system.
Common authentication methods include passwords, OAuth, and API keys.

## Password Authentication

Password authentication requires users to provide a secret passphrase.
Best practices include using strong passwords and enabling multi-factor authentication.

## OAuth

OAuth is an open standard for access delegation. It allows users to grant
third-party applications access to their resources without sharing credentials.
`)

	env.writeTestFile("api.go", `package api

import "net/http"

// HandleLogin processes user login requests.
// It validates credentials and returns an authentication token.
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Validate username and password
	// Generate JWT token
	// Return token in response
}

// HandleLogout invalidates the user's session token.
func HandleLogout(w http.ResponseWriter, r *http.Request) {
	// Clear session cookie
	// Invalidate token in store
}
`)

	result := env.runIndex(true)
	if result.FileCount < 2 {
		t.Fatalf("expected at least 2 files indexed, got %d", result.FileCount)
	}
	if result.ChunkCount == 0 {
		t.Fatal("expected chunks to be produced")
	}
	t.Logf("indexed %d files, %d chunks", result.FileCount, result.ChunkCount)

	results := env.runSearch("how to authenticate users", 5)
	if len(results) == 0 {
		t.Fatal("expected search results for authentication query")
	}

	found := false
	for _, r := range results {
		lower := strings.ToLower(r.Snippet)
		if strings.Contains(lower, "authentication") || strings.Contains(lower, "login") || strings.Contains(lower, "oauth") {
			found = true
			break
		}
	}
	if !found {
		t.Log("search results:")
		for i, r := range results {
			t.Logf("  %d: %s (score: %.4f)", i+1, r.FilePath, r.Score)
			t.Logf("      %s", r.Snippet[:min(100, len(r.Snippet))])
		}
		t.Fatal("no results matched authentication-related content")
	}
}

// TestIncrementalSync tests that modifying a file triggers re-indexing of only that file.
func TestIncrementalSync(t *testing.T) {
	skipIfNoONNX(t)

	env := setupTestEnv(t)

	env.writeTestFile("stable.md", "# Stable Document\n\nThis content will not change.")
	env.writeTestFile("changing.md", "# Initial Version\n\nThis will be updated.")

	initialResult := env.runIndex(true)
	if initialResult.FileCount != 2 {
		t.Fatalf("expected 2 files, got %d", initialResult.FileCount)
	}
	t.Logf("initial index: %d files, %d chunks", initialResult.FileCount, initialResult.ChunkCount)

	env.writeTestFile("changing.md", "# Updated Version\n\nThis content has been significantly changed with new information about deployment pipelines.")

	syncResult := env.runSync()
	t.Logf("sync: %d new, %d changed, %d deleted", syncResult.NewFiles, syncResult.ChangedFiles, syncResult.DeletedFiles)

	if syncResult.ChangedFiles == 0 && syncResult.NewFiles == 0 {
		t.Fatal("expected at least one changed or new file in incremental sync")
	}

	results := env.runSearch("deployment pipelines", 5)
	if len(results) == 0 {
		t.Fatal("expected results for 'deployment pipelines' after sync")
	}

	found := false
	for _, r := range results {
		if strings.Contains(strings.ToLower(r.Snippet), "deployment") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected to find deployment-related content after sync")
	}
}

// TestSearchModes tests that semantic search mode returns results.
func TestSearchModes(t *testing.T) {
	skipIfNoONNX(t)

	env := setupTestEnv(t)

	env.writeTestFile("database.md", `# Database Configuration

Database connection pooling improves performance by reusing connections.
Configure the pool size based on your workload.
Typical pool sizes range from 5 to 20 connections.
`)

	env.writeTestFile("main.go", `package main

// DatabasePool manages a pool of database connections.
// It handles connection lifecycle and health checks.
type DatabasePool struct {
	maxConnections int
	activeConnections int
}

// NewDatabasePool creates a new connection pool with the given size.
func NewDatabasePool(size int) *DatabasePool {
	return &DatabasePool{maxConnections: size}
}
`)

	env.runIndex(true)

	results := env.runSearch("database connection management", 5)
	if len(results) == 0 {
		t.Fatal("semantic search returned no results")
	}
	t.Logf("semantic search: %d results", len(results))

	for i, r := range results {
		if r.ChunkID == "" {
			t.Errorf("result %d: missing chunk_id", i)
		}
		if r.FilePath == "" {
			t.Errorf("result %d: missing file_path", i)
		}
		if r.Snippet == "" {
			t.Errorf("result %d: missing snippet", i)
		}
		if r.Score == 0 {
			t.Errorf("result %d: zero score", i)
		}
	}
}

// TestLanguageFilter tests filtering search results by programming language.
func TestLanguageFilter(t *testing.T) {
	skipIfNoONNX(t)

	env := setupTestEnv(t)

	env.writeTestFile("notes.md", "# Project Notes\n\nWe use Go for the backend and Python for data processing.")
	env.writeTestFile("server.go", `package server

// StartServer initializes and starts the HTTP server.
func StartServer(port int) error {
	return nil
}
`)
	env.writeTestFile("process.py", `# Data processing pipeline
def process_data(input_path):
    """Read and transform the input data."""
    pass
`)

	env.runIndex(true)

	allResults := env.runSearch("initialization and setup", 10)
	if len(allResults) == 0 {
		t.Fatal("expected results before filtering")
	}

	goResults := env.runSearchWithFilters("initialization and setup", 10, "go", "", "")
	for _, r := range goResults {
		if r.Metadata.Language != "go" {
			t.Errorf("expected language 'go', got %q in %s", r.Metadata.Language, r.FilePath)
		}
	}

	pyResults := env.runSearchWithFilters("initialization and setup", 10, "python", "", "")
	for _, r := range pyResults {
		if r.Metadata.Language != "python" {
			t.Errorf("expected language 'python', got %q in %s", r.Metadata.Language, r.FilePath)
		}
	}

	mdResults := env.runSearchWithFilters("initialization and setup", 10, "markdown", "", "")
	for _, r := range mdResults {
		if r.Metadata.Language != "markdown" {
			t.Errorf("expected language 'markdown', got %q in %s", r.Metadata.Language, r.FilePath)
		}
	}

	t.Logf("all: %d, go: %d, python: %d, markdown: %d", len(allResults), len(goResults), len(pyResults), len(mdResults))
}

// TestKindFilter tests filtering search results by file kind.
func TestKindFilter(t *testing.T) {
	skipIfNoONNX(t)

	env := setupTestEnv(t)

	env.writeTestFile("readme.md", "# README\n\nThis is the project readme with setup instructions.")
	env.writeTestFile("app.go", `package app

// Initialize sets up the application configuration.
func Initialize() error {
	return nil
}
`)

	env.runIndex(true)

	codeResults := env.runSearchWithFilters("setup and configuration", 10, "", "code", "")
	for _, r := range codeResults {
		if r.Metadata.FileKind != "code" {
			t.Errorf("expected kind 'code', got %q in %s", r.Metadata.FileKind, r.FilePath)
		}
	}

	mdResults := env.runSearchWithFilters("setup and configuration", 10, "", "markdown", "")
	for _, r := range mdResults {
		if r.Metadata.FileKind != "markdown" {
			t.Errorf("expected kind 'markdown', got %q in %s", r.Metadata.FileKind, r.FilePath)
		}
	}

	t.Logf("code: %d, markdown: %d", len(codeResults), len(mdResults))
}

// TestDirFilter tests filtering search results by subdirectory.
func TestDirFilter(t *testing.T) {
	skipIfNoONNX(t)

	env := setupTestEnv(t)

	env.writeTestFile("src/main.go", `package main

// Main entry point for the application.
func main() {}
`)
	env.writeTestFile("docs/guide.md", "# User Guide\n\nThis guide explains how to use the application.")

	env.runIndex(true)

	srcResults := env.runSearchWithFilters("application", 10, "", "", "src/")
	for _, r := range srcResults {
		if !strings.Contains(r.FilePath, "src/") {
			t.Errorf("expected path to contain 'src/', got %s", r.FilePath)
		}
	}

	docsResults := env.runSearchWithFilters("application", 10, "", "", "docs/")
	for _, r := range docsResults {
		if !strings.Contains(r.FilePath, "docs/") {
			t.Errorf("expected path to contain 'docs/', got %s", r.FilePath)
		}
	}

	t.Logf("src/: %d, docs/: %d", len(srcResults), len(docsResults))
}

// TestJSONOutput verifies that search results can be serialized to valid JSON
// with the expected fields.
func TestJSONOutput(t *testing.T) {
	skipIfNoONNX(t)

	env := setupTestEnv(t)

	env.writeTestFile("config.md", `# Configuration

The application reads configuration from environment variables.
Set DATABASE_URL to configure the database connection.
Set API_KEY to configure external API access.
`)

	env.runIndex(true)
	results := env.runSearch("environment variables configuration", 5)

	if len(results) == 0 {
		t.Fatal("expected search results")
	}

	response := output.SearchResponse{
		Query:   "environment variables configuration",
		Mode:    "semantic",
		Results: results,
		Total:   len(results),
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal to JSON: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal JSON: %v", err)
	}

	expectedFields := []string{"query", "mode", "results", "total"}
	for _, field := range expectedFields {
		if _, ok := parsed[field]; !ok {
			t.Errorf("missing JSON field: %s", field)
		}
	}

	resultsArr, ok := parsed["results"].([]interface{})
	if !ok {
		t.Fatal("results is not an array")
	}
	if len(resultsArr) == 0 {
		t.Fatal("results array is empty")
	}

	firstResult, ok := resultsArr[0].(map[string]interface{})
	if !ok {
		t.Fatal("first result is not an object")
	}
	resultFields := []string{"chunk_id", "file_path", "snippet", "score", "source_name", "metadata"}
	for _, field := range resultFields {
		if _, ok := firstResult[field]; !ok {
			t.Errorf("missing result field: %s", field)
		}
	}

	t.Logf("JSON output valid: %d results, %d bytes", len(resultsArr), len(data))
}

// TestSourceFilter tests that --source filter restricts indexing to one source.
func TestSourceFilter(t *testing.T) {
	skipIfNoONNX(t)

	env := setupTestEnv(t)

	source2Dir := filepath.Join(env.tmpDir, "source2")
	if err := os.MkdirAll(source2Dir, 0o755); err != nil {
		t.Fatalf("create source2 dir: %v", err)
	}

	env.writeTestFile("file1.md", "# Source One\n\nContent from the first source about authentication.")

	if err := os.WriteFile(filepath.Join(source2Dir, "file2.md"), []byte("# Source Two\n\nContent from the second source about deployment."), 0o644); err != nil {
		t.Fatalf("write source2 file: %v", err)
	}

	env.cfg.Sources = append(env.cfg.Sources, config.SourceConfig{
		Name:              "source-two",
		Path:              source2Dir,
		Enabled:           true,
		IncludeExtensions: []string{"md", "go", "py", "yaml", "yml", "json", "txt", "toml", "rs", "ts", "js"},
	})

	ctx := context.Background()
	result, err := indexer.Run(ctx, env.paths, env.cfg, "test-source", true, env.logger, nil)
	if err != nil {
		t.Fatalf("index with source filter: %v", err)
	}

	if result.SourceCount != 1 {
		t.Errorf("expected 1 source indexed, got %d", result.SourceCount)
	}

	results := env.runSearch("authentication", 5)
	for _, r := range results {
		if r.SourceName != "test-source" {
			t.Errorf("expected source 'test-source', got %q", r.SourceName)
		}
	}

	t.Logf("source-filtered index: %d sources, %d files", result.SourceCount, result.FileCount)
}

// TestEmptySearch tests that searching an empty index returns no results gracefully.
func TestEmptySearch(t *testing.T) {
	skipIfNoONNX(t)

	env := setupTestEnv(t)

	ctx := context.Background()
	_, err := indexer.Run(ctx, env.paths, env.cfg, "", true, env.logger, nil)
	if err != nil {
		t.Logf("index with no files returned: %v", err)
		return
	}

	results := env.runSearch("anything", 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty index, got %d", len(results))
	}
}

// TestMultipleFileTypes tests indexing and searching across different file types.
func TestMultipleFileTypes(t *testing.T) {
	skipIfNoONNX(t)

	env := setupTestEnv(t)

	env.writeTestFile("readme.md", `# API Documentation

The REST API provides endpoints for user management.
Use POST /users to create a new user account.
Use GET /users/:id to retrieve user details.
`)

	env.writeTestFile("handler.go", `package handler

// CreateUser handles POST /users requests.
// It validates the request body and creates a new user in the database.
func CreateUser(w http.ResponseWriter, r *http.Request) {
	// Parse JSON body
	// Validate fields
	// Insert into database
	// Return 201 Created
}

// GetUser handles GET /users/:id requests.
func GetUser(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from path
	// Query database
	// Return user JSON
}
`)

	env.writeTestFile("config.yaml", `# Server configuration
server:
  port: 8080
  host: localhost
database:
  url: postgres://localhost:5432/mydb
  max_connections: 10
`)

	env.runIndex(true)

	results := env.runSearch("create user endpoint", 10)
	if len(results) == 0 {
		t.Fatal("expected results for 'create user endpoint'")
	}

	var foundMD, foundGo bool
	for _, r := range results {
		if r.Metadata.FileKind == "markdown" {
			foundMD = true
		}
		if r.Metadata.FileKind == "code" {
			foundGo = true
		}
	}

	if !foundMD {
		t.Log("did not find markdown results")
	}
	if !foundGo {
		t.Log("did not find code results")
	}

	t.Logf("found markdown: %v, code: %v, total: %d", foundMD, foundGo, len(results))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
