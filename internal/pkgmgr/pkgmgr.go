package pkgmgr

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"sophonie/sono/internal/config"
)

const (
	registryBase = "https://registry.npmjs.org/"
	packumentTTL = time.Hour
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

type PackageManager struct {
	Name     string
	Registry string
}

var Supported = []PackageManager{
	{Name: "pnpm", Registry: "pnpm"},
	{Name: "yarn", Registry: "yarn"},
}

func Find(name string) (PackageManager, bool) {
	for _, pm := range Supported {
		if pm.Name == name {
			return pm, true
		}
	}
	return PackageManager{}, false
}

type packument struct {
	DistTags map[string]string `json:"dist-tags"`
	Versions map[string]struct {
		Dist struct {
			Tarball   string `json:"tarball"`
			Integrity string `json:"integrity"`
		} `json:"dist"`
	} `json:"versions"`
}

func ListStableVersions(cfg *config.Config, pm PackageManager) ([]string, error) {
	doc, err := loadPackument(cfg, pm)
	if err != nil {
		return nil, err
	}

	var out []string
	for version := range doc.Versions {
		if strings.Contains(version, "-") {
			continue
		}
		if _, ok := parseSemver(version); ok {
			out = append(out, version)
		}
	}
	sort.Slice(out, func(i, j int) bool { return greaterSemver(out[i], out[j]) })
	return out, nil
}

func Install(cfg *config.Config, pm PackageManager, version string) error {
	doc, err := loadPackument(cfg, pm)
	if err != nil {
		return err
	}
	entry, ok := doc.Versions[version]
	if !ok {
		return fmt.Errorf("unknown %s version: %s", pm.Name, version)
	}

	data, err := downloadBytes(entry.Dist.Tarball)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if err := verifyIntegrity(data, entry.Dist.Integrity); err != nil {
		return err
	}
	return extractTgz(data, versionDir(cfg, pm, version))
}

func Installed(cfg *config.Config, pm PackageManager) ([]string, error) {
	entries, err := os.ReadDir(pmRoot(cfg, pm))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}
	return versions, nil
}

func Active(cfg *config.Config, pm PackageManager) (string, error) {
	target, err := os.Readlink(currentLink(cfg, pm))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return filepath.Base(target), nil
}

func Activate(cfg *config.Config, pm PackageManager, version string) error {
	dir := versionDir(cfg, pm, version)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("%s version not installed: %s", pm.Name, version)
	}

	bins, err := readBin(dir)
	if err != nil {
		return err
	}
	for command, entry := range bins {
		nodeBin := filepath.Join(cfg.CurrentSymlink, "bin", "node")
		target := filepath.Join(dir, entry)
		script := fmt.Sprintf("#!/bin/sh\nexec %q %q \"$@\"\n", nodeBin, target)
		if err := os.WriteFile(filepath.Join(cfg.ShimsDir, command), []byte(script), 0o755); err != nil {
			return err
		}
	}

	link := currentLink(cfg, pm)
	tmp := link + ".tmp"
	os.Remove(tmp)
	if err := os.Symlink(version, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, link)
}

func Uninstall(cfg *config.Config, pm PackageManager, version string) error {
	active, err := Active(cfg, pm)
	if err != nil {
		return err
	}
	if version == active {
		return fmt.Errorf("cannot remove the active %s version %s; activate another one first", pm.Name, version)
	}
	return os.RemoveAll(versionDir(cfg, pm, version))
}

func readBin(dir string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, err
	}

	var pkg struct {
		Name string          `json:"name"`
		Bin  json.RawMessage `json:"bin"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	bins := map[string]string{}
	if err := json.Unmarshal(pkg.Bin, &bins); err == nil {
		return bins, nil
	}
	var single string
	if err := json.Unmarshal(pkg.Bin, &single); err == nil {
		name := pkg.Name
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		return map[string]string{name: single}, nil
	}
	return nil, fmt.Errorf("no runnable command found in %s package.json", filepath.Base(dir))
}

func loadPackument(cfg *config.Config, pm PackageManager) (*packument, error) {
	cachePath := packumentPath(cfg, pm)
	if cacheFresh(cachePath) {
		if data, err := os.ReadFile(cachePath); err == nil {
			if doc, err := parsePackument(data); err == nil {
				return doc, nil
			}
		}
	}

	data, err := fetchPackument(pm)
	if err != nil {
		if cached, readErr := os.ReadFile(cachePath); readErr == nil {
			if doc, parseErr := parsePackument(cached); parseErr == nil {
				return doc, nil
			}
		}
		return nil, err
	}
	_ = os.WriteFile(cachePath, data, 0o644)
	return parsePackument(data)
}

func fetchPackument(pm PackageManager) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, registryBase+pm.Registry, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.npm.install-v1+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned %s for %s", resp.Status, pm.Name)
	}
	return io.ReadAll(resp.Body)
}

func parsePackument(data []byte) (*packument, error) {
	var doc packument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func downloadBytes(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func verifyIntegrity(data []byte, integrity string) error {
	if !strings.HasPrefix(integrity, "sha512-") {
		return fmt.Errorf("unsupported integrity format: %s", integrity)
	}
	sum := sha512.Sum512(data)
	got := base64.StdEncoding.EncodeToString(sum[:])
	if got != strings.TrimPrefix(integrity, "sha512-") {
		return fmt.Errorf("integrity mismatch")
	}
	return nil
}

func extractTgz(data []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)

	tmp := dest + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}

	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			os.RemoveAll(tmp)
			return err
		}

		rel := stripFirstComponent(header.Name)
		if rel == "" {
			continue
		}
		target := filepath.Join(tmp, rel)
		if !strings.HasPrefix(target, filepath.Clean(tmp)+string(os.PathSeparator)) {
			os.RemoveAll(tmp)
			return fmt.Errorf("invalid archive path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				os.RemoveAll(tmp)
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				os.RemoveAll(tmp)
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				os.RemoveAll(tmp)
				return err
			}
			if _, err := io.Copy(out, reader); err != nil {
				out.Close()
				os.RemoveAll(tmp)
				return err
			}
			out.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				os.RemoveAll(tmp)
				return err
			}
			os.Remove(target)
			if err := os.Symlink(header.Linkname, target); err != nil {
				os.RemoveAll(tmp)
				return err
			}
		}
	}

	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

func stripFirstComponent(name string) string {
	name = strings.TrimPrefix(name, "./")
	index := strings.Index(name, "/")
	if index < 0 {
		return ""
	}
	return name[index+1:]
}

func packumentPath(cfg *config.Config, pm PackageManager) string {
	return filepath.Join(cfg.PmDir, pm.Name+".json")
}

func pmRoot(cfg *config.Config, pm PackageManager) string {
	return filepath.Join(cfg.PmDir, pm.Name)
}

func versionDir(cfg *config.Config, pm PackageManager, version string) string {
	return filepath.Join(pmRoot(cfg, pm), version)
}

func currentLink(cfg *config.Config, pm PackageManager) string {
	return filepath.Join(pmRoot(cfg, pm), "current")
}

func cacheFresh(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < packumentTTL
}

type semver struct{ major, minor, patch int }

func parseSemver(version string) (semver, bool) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return semver{}, false
	}
	return semver{major, minor, patch}, true
}

func greaterSemver(a, b string) bool {
	sa, oka := parseSemver(a)
	sb, okb := parseSemver(b)
	if !oka || !okb {
		return a > b
	}
	if sa.major != sb.major {
		return sa.major > sb.major
	}
	if sa.minor != sb.minor {
		return sa.minor > sb.minor
	}
	return sa.patch > sb.patch
}
