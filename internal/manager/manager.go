package manager

import (
	"archive/tar"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ulikunitz/xz"

	"sophonie/sono/internal/config"
	"sophonie/sono/internal/nodedist"
)

const (
	StageDownloading = "downloading"
	StageVerifying   = "verifying"
	StageExtracting  = "extracting"
)

func ListInstalled(cfg *config.Config) ([]string, error) {
	entries, err := os.ReadDir(cfg.VersionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var versions []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "v") {
			versions = append(versions, entry.Name())
		}
	}
	return versions, nil
}

func Active(cfg *config.Config) (string, error) {
	target, err := os.Readlink(cfg.CurrentSymlink)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return filepath.Base(target), nil
}

func ResolvedOnPath() string {
	binary, err := exec.LookPath("node")
	if err != nil {
		return ""
	}
	output, err := exec.Command(binary, "-v").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func CacheInfo(cfg *config.Config) (int, int64) {
	entries, err := os.ReadDir(cfg.CacheDir)
	if err != nil {
		return 0, 0
	}

	var count int
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.xz") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		count++
		total += info.Size()
	}
	return count, total
}

func PurgeCache(cfg *config.Config) (int, error) {
	return purgeTarballs(cfg, func(os.FileInfo) bool { return true })
}

func PurgeExpired(cfg *config.Config, maxAgeDays int) (int, error) {
	cutoff := time.Now().Add(-time.Duration(maxAgeDays) * 24 * time.Hour)
	return purgeTarballs(cfg, func(info os.FileInfo) bool {
		return info.ModTime().Before(cutoff)
	})
}

func purgeTarballs(cfg *config.Config, shouldRemove func(os.FileInfo) bool) (int, error) {
	entries, err := os.ReadDir(cfg.CacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.xz") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !shouldRemove(info) {
			continue
		}
		if os.Remove(filepath.Join(cfg.CacheDir, entry.Name())) == nil {
			count++
		}
	}
	return count, nil
}

func AvailableUpdates(cfg *config.Config, index nodedist.Index) map[string]string {
	installed, err := ListInstalled(cfg)
	if err != nil {
		return nil
	}

	installedSet := map[string]bool{}
	for _, version := range installed {
		installedSet[version] = true
	}

	updates := map[string]string{}
	for _, version := range installed {
		latest := index.LatestInMinor(version)
		if latest != "" && latest != version && !installedSet[latest] {
			updates[version] = latest
		}
	}
	return updates
}

func DirOnPath(dir string) bool {
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if entry == dir {
			return true
		}
	}
	return false
}

func CurrentBinOnPath(cfg *config.Config) bool {
	return DirOnPath(filepath.Join(cfg.CurrentSymlink, "bin"))
}

func SetActive(cfg *config.Config, version string) error {
	info, err := os.Stat(filepath.Join(cfg.VersionsDir, version))
	if err != nil || !info.IsDir() {
		return fmt.Errorf("version not installed: %s", version)
	}

	tmp := cfg.CurrentSymlink + ".tmp"
	os.Remove(tmp)
	target := filepath.Join(filepath.Base(cfg.VersionsDir), version)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, cfg.CurrentSymlink)
}

func Uninstall(cfg *config.Config, version string) error {
	active, err := Active(cfg)
	if err != nil {
		return err
	}
	if version == active {
		return fmt.Errorf("cannot remove the active version %s; activate another one first", version)
	}
	return os.RemoveAll(filepath.Join(cfg.VersionsDir, version))
}

func Install(cfg *config.Config, version string, progress func(stage string, downloaded, total int64)) error {
	platform, err := config.Platform()
	if err != nil {
		return err
	}

	filename := nodedist.TarballName(version, platform)
	tarballPath := filepath.Join(cfg.CacheDir, filename)

	checksums, err := nodedist.FetchChecksums(version)
	if err != nil {
		return fmt.Errorf("fetching checksums: %w", err)
	}
	expected, ok := checksums[filename]
	if !ok {
		return fmt.Errorf("no checksum for %s", filename)
	}

	if !tarballMatches(tarballPath, expected) {
		progress(StageDownloading, 0, 0)
		sum, err := nodedist.Download(version, platform, tarballPath, func(downloaded, total int64) {
			progress(StageDownloading, downloaded, total)
		})
		if err != nil {
			os.Remove(tarballPath)
			return fmt.Errorf("download failed: %w", err)
		}
		progress(StageVerifying, 0, 0)
		if sum != expected {
			os.Remove(tarballPath)
			return fmt.Errorf("checksum mismatch for %s (expected %s, got %s)", filename, expected, sum)
		}
	} else {
		now := time.Now()
		_ = os.Chtimes(tarballPath, now, now)
		progress(StageVerifying, 0, 0)
	}

	progress(StageExtracting, 0, 0)
	if err := extractTarXz(tarballPath, cfg.VersionsDir, version); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}
	return nil
}

func tarballMatches(path, expected string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}
	return hex.EncodeToString(hash.Sum(nil)) == expected
}

func extractTarXz(archivePath, versionsDir, version string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	xzReader, err := xz.NewReader(bufio.NewReader(file))
	if err != nil {
		return err
	}
	tarReader := tar.NewReader(xzReader)

	tmpDir := filepath.Join(versionsDir, version+".tmp")
	if err := os.RemoveAll(tmpDir); err != nil {
		return err
	}
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return err
	}

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			os.RemoveAll(tmpDir)
			return err
		}

		relPath := stripFirstComponent(header.Name)
		if relPath == "" {
			continue
		}
		target := filepath.Join(tmpDir, relPath)
		if !strings.HasPrefix(target, filepath.Clean(tmpDir)+string(os.PathSeparator)) {
			os.RemoveAll(tmpDir)
			return fmt.Errorf("invalid archive path: %s", header.Name)
		}
		if err := extractEntry(tarReader, header, tmpDir, target); err != nil {
			os.RemoveAll(tmpDir)
			return err
		}
	}

	finalDir := filepath.Join(versionsDir, version)
	if err := os.RemoveAll(finalDir); err != nil {
		return err
	}
	return os.Rename(tmpDir, finalDir)
}

func extractEntry(tarReader *tar.Reader, header *tar.Header, root, target string) error {
	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, 0o755)
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, tarReader)
		return err
	case tar.TypeSymlink:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		os.Remove(target)
		return os.Symlink(header.Linkname, target)
	case tar.TypeLink:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		os.Remove(target)
		return os.Link(filepath.Join(root, stripFirstComponent(header.Linkname)), target)
	default:
		return nil
	}
}

func stripFirstComponent(name string) string {
	name = strings.TrimPrefix(name, "./")
	index := strings.Index(name, "/")
	if index < 0 {
		return ""
	}
	return name[index+1:]
}
