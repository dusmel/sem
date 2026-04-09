package config

import "path/filepath"

type Config struct {
	General   GeneralConfig   `mapstructure:"general" toml:"general"`
	Embedding EmbeddingConfig `mapstructure:"embedding" toml:"embedding"`
	Storage   StorageConfig   `mapstructure:"storage" toml:"storage"`
	Chunking  ChunkingConfig  `mapstructure:"chunking" toml:"chunking"`
	Ignore    IgnoreConfig    `mapstructure:"ignore" toml:"ignore"`
	Sources   []SourceConfig  `mapstructure:"sources" toml:"sources"`
}

type GeneralConfig struct {
	DefaultBundle string `mapstructure:"default_bundle" toml:"default_bundle"`
	EmbeddingMode string `mapstructure:"embedding_mode" toml:"embedding_mode"`
}

type EmbeddingConfig struct {
	Mode          string `mapstructure:"mode" toml:"mode"`
	ModelCacheDir string `mapstructure:"model_cache_dir" toml:"model_cache_dir"`
	BatchSize     int    `mapstructure:"batch_size" toml:"batch_size"`
	MaxTokens     int    `mapstructure:"max_tokens" toml:"max_tokens"`
	Normalize     bool   `mapstructure:"normalize" toml:"normalize"`
}

type StorageConfig struct {
	BundleDir string               `mapstructure:"bundle_dir" toml:"bundle_dir"`
	Backend   string               `mapstructure:"backend" toml:"backend"`
	LanceDB   LanceDBStorageConfig `mapstructure:"lancedb" toml:"lancedb"`
}

type LanceDBStorageConfig struct {
	Path   string `mapstructure:"path" toml:"path"`
	Table  string `mapstructure:"table" toml:"table"`
	Metric string `mapstructure:"metric" toml:"metric"`
}

type ChunkingConfig struct {
	MaxChars        int  `mapstructure:"max_chars" toml:"max_chars"`
	OverlapChars    int  `mapstructure:"overlap_chars" toml:"overlap_chars"`
	MinChars        int  `mapstructure:"min_chars" toml:"min_chars"`
	RespectHeadings bool `mapstructure:"respect_headings" toml:"respect_headings"`
}

type IgnoreConfig struct {
	DefaultPatterns []string `mapstructure:"default_patterns" toml:"default_patterns"`
	UseGitignore    bool     `mapstructure:"use_gitignore" toml:"use_gitignore"`
}

type SourceConfig struct {
	Name              string   `mapstructure:"name" toml:"name" json:"name"`
	Path              string   `mapstructure:"path" toml:"path" json:"path"`
	Enabled           bool     `mapstructure:"enabled" toml:"enabled" json:"enabled"`
	IncludeExtensions []string `mapstructure:"include_extensions" toml:"include_extensions" json:"include_extensions"`
	ExcludePatterns   []string `mapstructure:"exclude_patterns" toml:"exclude_patterns" json:"exclude_patterns"`
}

func Default(baseDir string) Config {
	return Config{
		General: GeneralConfig{
			DefaultBundle: "default",
			// EmbeddingMode is deprecated; use Embedding.Mode instead
		},
		Embedding: EmbeddingConfig{
			Mode:          "balanced",
			ModelCacheDir: filepath.Join(baseDir, "models"),
			BatchSize:     32,
			MaxTokens:     512,
			Normalize:     true,
		},
		Storage: StorageConfig{
			BundleDir: filepath.Join(baseDir, "bundles"),
			Backend:   "lancedb",
			LanceDB: LanceDBStorageConfig{
				Path:   filepath.Join(baseDir, "backends", "lancedb"),
				Table:  "chunks",
				Metric: "cosine",
			},
		},
		Chunking: ChunkingConfig{
			MaxChars:        2200,
			OverlapChars:    300,
			MinChars:        400,
			RespectHeadings: true,
		},
		Ignore: IgnoreConfig{
			DefaultPatterns: []string{
				".git",
				"node_modules",
				"target",
				"dist",
				"build",
				"vendor",
				"__pycache__",
				".idea",
				".vscode",
				".obsidian",
				"*.min.js",
				"*.min.css",
			},
			UseGitignore: true,
		},
		Sources: []SourceConfig{},
	}
}
