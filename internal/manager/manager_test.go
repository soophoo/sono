package manager

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophonie/sono/internal/config"
	"sophonie/sono/internal/nodedist"
)

func TestInstallFailsOnChecksumMismatch(t *testing.T) {
	platform, err := config.Platform()
	if err != nil {
		t.Skipf("unsupported platform: %v", err)
	}

	version := "v0.0.0-test"
	tarball := nodedist.TarballName(version, platform)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "SHASUMS256.txt") {
			fmt.Fprintf(w, "%s  %s\n", strings.Repeat("0", 64), tarball)
			return
		}
		w.Write([]byte("ceci n'est pas un vrai tarball"))
	}))
	defer server.Close()

	original := nodedist.DistBase
	nodedist.DistBase = server.URL
	defer func() { nodedist.DistBase = original }()

	root := t.TempDir()
	cfg := &config.Config{
		CacheDir:    filepath.Join(root, "cache"),
		VersionsDir: filepath.Join(root, "versions"),
	}
	os.MkdirAll(cfg.CacheDir, 0o755)
	os.MkdirAll(cfg.VersionsDir, 0o755)

	err = Install(cfg, version, func(string, int64, int64) {})
	if err == nil {
		t.Fatal("expected a checksum error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(cfg.CacheDir, tarball)); !os.IsNotExist(statErr) {
		t.Fatal("the corrupted tarball should have been removed")
	}
	if _, statErr := os.Stat(filepath.Join(cfg.VersionsDir, version)); !os.IsNotExist(statErr) {
		t.Fatal("no version directory should be created after a checksum failure")
	}
}
