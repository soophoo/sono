package nodedist

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var DistBase = "https://nodejs.org/dist"

var downloadClient = &http.Client{Timeout: 10 * time.Minute}

func TarballName(version, platform string) string {
	return fmt.Sprintf("node-%s-%s.tar.xz", version, platform)
}

func FetchChecksums(version string) (map[string]string, error) {
	data, err := fetchURL(fmt.Sprintf("%s/%s/SHASUMS256.txt", DistBase, version))
	if err != nil {
		return nil, err
	}

	checksums := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			checksums[fields[1]] = fields[0]
		}
	}
	return checksums, nil
}

func Download(version, platform, dest string, progress func(downloaded, total int64)) (string, error) {
	url := fmt.Sprintf("%s/%s/%s", DistBase, version, TarballName(version, platform))
	resp, err := downloadClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s returned %s", url, resp.Status)
	}

	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()

	hash := sha256.New()
	writer := io.MultiWriter(out, hash)
	buf := make([]byte, 64*1024)
	var downloaded int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := writer.Write(buf[:n]); err != nil {
				return "", err
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, resp.ContentLength)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
