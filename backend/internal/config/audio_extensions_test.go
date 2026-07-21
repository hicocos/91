package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultScannerAudioExtensions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	for _, want := range []string{".mp3", ".m4a", ".aac", ".flac", ".wav", ".ogg", ".oga", ".opus"} {
		if !hasVideoExtension(cfg.Scanner.AudioExtensions, want) {
			t.Fatalf("audio extensions = %#v, want %s", cfg.Scanner.AudioExtensions, want)
		}
	}
}

func TestLoadCustomScannerAudioExtensionsArePreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("scanner:\n  audio_extensions: [\".mp3\"]\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Scanner.AudioExtensions) != 1 || cfg.Scanner.AudioExtensions[0] != ".mp3" {
		t.Fatalf("audio extensions = %#v, want custom list preserved", cfg.Scanner.AudioExtensions)
	}
}
