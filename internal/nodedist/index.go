package nodedist

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	indexURL = "https://nodejs.org/dist/index.json"
	cacheTTL = time.Hour
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type LTSFlag struct {
	IsLTS    bool
	Codename string
}

func (l *LTSFlag) UnmarshalJSON(data []byte) error {
	if string(data) == "false" {
		l.IsLTS = false
		l.Codename = ""
		return nil
	}
	var codename string
	if err := json.Unmarshal(data, &codename); err != nil {
		return err
	}
	l.IsLTS = true
	l.Codename = codename
	return nil
}

type Release struct {
	Version string   `json:"version"`
	Date    string   `json:"date"`
	Files   []string `json:"files"`
	LTS     LTSFlag  `json:"lts"`
}

type Index []Release

func FetchIndex(cachePath string) (Index, error) {
	data, err := fetchURL(indexURL)
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile(cachePath, data, 0o644)
	return parseIndex(data)
}

func LoadIndex(cachePath string) (Index, error) {
	data, err := cachedBytes(cachePath, indexURL)
	if err != nil {
		return nil, err
	}
	return parseIndex(data)
}

func fetchURL(url string) ([]byte, error) {
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

func cachedBytes(cachePath, url string) ([]byte, error) {
	if cacheFresh(cachePath) {
		if data, err := os.ReadFile(cachePath); err == nil {
			return data, nil
		}
	}
	data, err := fetchURL(url)
	if err != nil {
		if cached, readErr := os.ReadFile(cachePath); readErr == nil {
			return cached, nil
		}
		return nil, err
	}
	_ = os.WriteFile(cachePath, data, 0o644)
	return data, nil
}

func parseIndex(data []byte) (Index, error) {
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	return index, nil
}

func cacheFresh(cachePath string) bool {
	info, err := os.Stat(cachePath)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < cacheTTL
}

func (idx Index) LTS() Index {
	var out Index
	for _, release := range idx {
		if release.LTS.IsLTS {
			out = append(out, release)
		}
	}
	return out
}

func (idx Index) NonLTS() Index {
	var out Index
	for _, release := range idx {
		if !release.LTS.IsLTS {
			out = append(out, release)
		}
	}
	return out
}

func (idx Index) SearchPrefix(query string) Index {
	prefix := strings.TrimPrefix(strings.TrimSpace(query), "v")
	if prefix == "" {
		return idx
	}
	var out Index
	for _, release := range idx {
		if strings.HasPrefix(strings.TrimPrefix(release.Version, "v"), prefix) {
			out = append(out, release)
		}
	}
	return out
}

func (idx Index) LatestPerMajor() Index {
	latest := map[int]Release{}
	for _, release := range idx {
		version, err := parseVersion(release.Version)
		if err != nil {
			continue
		}
		current, seen := latest[version.major]
		if !seen || compareVersions(release.Version, current.Version) > 0 {
			latest[version.major] = release
		}
	}
	out := make(Index, 0, len(latest))
	for _, release := range latest {
		out = append(out, release)
	}
	return out.Sorted()
}

func (idx Index) LatestInMinor(version string) string {
	target, err := parseVersion(version)
	if err != nil {
		return ""
	}
	best := ""
	for _, release := range idx {
		v, err := parseVersion(release.Version)
		if err != nil || v.major != target.major || v.minor != target.minor {
			continue
		}
		if best == "" || compareVersions(release.Version, best) > 0 {
			best = release.Version
		}
	}
	return best
}

func (idx Index) LatestLTS() string {
	best := ""
	for _, release := range idx {
		if !release.LTS.IsLTS {
			continue
		}
		if best == "" || compareVersions(release.Version, best) > 0 {
			best = release.Version
		}
	}
	return best
}

func (idx Index) Sorted() Index {
	out := make(Index, len(idx))
	copy(out, idx)
	sort.SliceStable(out, func(i, j int) bool {
		return compareVersions(out[i].Version, out[j].Version) > 0
	})
	return out
}

type semver struct {
	major, minor, patch int
}

func parseVersion(version string) (semver, error) {
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("invalid version: %s", version)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semver{}, err
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semver{}, err
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semver{}, err
	}
	return semver{major, minor, patch}, nil
}

func compareVersions(a, b string) int {
	versionA, errA := parseVersion(a)
	versionB, errB := parseVersion(b)
	switch {
	case errA != nil && errB != nil:
		return strings.Compare(a, b)
	case errA != nil:
		return -1
	case errB != nil:
		return 1
	}
	if versionA.major != versionB.major {
		return versionA.major - versionB.major
	}
	if versionA.minor != versionB.minor {
		return versionA.minor - versionB.minor
	}
	return versionA.patch - versionB.patch
}
