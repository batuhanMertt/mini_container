// Package release resolves, downloads and caches the published Linux runtime
// binary that matches a given architecture.
package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// Owner and Repo identify where the runtime binaries are published.
	Owner = "batuhanMertt"
	Repo  = "mini_container"

	checksumsAsset = "checksums.txt"
	httpTimeout    = 60 * time.Second
)

// AssetName is the published name of the Linux runtime for an architecture.
func AssetName(arch string) string {
	return fmt.Sprintf("mini-container_linux_%s", arch)
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type release struct {
	Tag    string  `json:"tag_name"`
	Assets []asset `json:"assets"`
}

// Resolved is a runtime binary ready on the local filesystem.
type Resolved struct {
	Tag      string
	Asset    string
	Path     string
	Cached   bool // true when the download was skipped
	Size     int64
	Verified bool // true when a checksums.txt entry matched
}

// Fetch resolves the release binary for arch, downloading it into the user
// cache the first time and reusing it afterwards.
//
// tag selects a specific release; an empty tag means "latest".
func Fetch(arch, tag string, progress io.Writer) (*Resolved, error) {
	client := &http.Client{Timeout: httpTimeout}

	rel, err := fetchRelease(client, tag)
	if err != nil {
		return nil, err
	}

	want := AssetName(arch)
	bin := findAsset(rel, want)
	if bin == nil {
		return nil, fmt.Errorf("release %s has no asset %q (published: %s)\n"+
			"push a tag so the release workflow builds it: git tag v0.1.0 && git push origin v0.1.0",
			rel.Tag, want, assetNames(rel))
	}

	dir := filepath.Join(cacheRoot(), rel.Tag)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, want)

	sums := fetchChecksums(client, rel)
	wantSum := sums[want]

	if sum, err := fileSum(path); err == nil && (wantSum == "" || sum == wantSum) {
		fi, _ := os.Stat(path)
		return &Resolved{Tag: rel.Tag, Asset: want, Path: path, Cached: true,
			Size: sizeOf(fi), Verified: wantSum != ""}, nil
	}

	fmt.Fprintf(progress, "  downloading %s (%s) from release %s\n", want, humanSize(bin.Size), rel.Tag)
	if err := download(client, bin.URL, path); err != nil {
		return nil, err
	}

	verified := false
	if wantSum != "" {
		got, err := fileSum(path)
		if err != nil {
			return nil, err
		}
		if got != wantSum {
			os.Remove(path)
			return nil, fmt.Errorf("checksum mismatch for %s: got %s, want %s", want, got, wantSum)
		}
		verified = true
	}

	fi, _ := os.Stat(path)
	return &Resolved{Tag: rel.Tag, Asset: want, Path: path, Size: sizeOf(fi), Verified: verified}, nil
}

func cacheRoot() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "mini-container")
}

// CacheDir is the directory downloads land in; used for the free-space probe.
func CacheDir() string {
	dir := cacheRoot()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return os.TempDir()
	}
	return dir
}

func newRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mini-container-launcher")
	// Anonymous GitHub API calls are rate limited to 60/hour per IP; a token
	// raises that but is never required.
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return req, nil
}

func fetchRelease(client *http.Client, tag string) (*release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", Owner, Repo)
	if tag != "" {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", Owner, Repo, tag)
	}

	req, err := newRequest(url)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting GitHub: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		if tag != "" {
			return nil, fmt.Errorf("release %q not found in %s/%s", tag, Owner, Repo)
		}
		return nil, fmt.Errorf("%s/%s has no published release yet;\n"+
			"push a tag to build one: git tag v0.1.0 && git push origin v0.1.0", Owner, Repo)
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, fmt.Errorf("GitHub API rate limit reached; set GITHUB_TOKEN and retry")
	default:
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decoding release metadata: %w", err)
	}
	return &rel, nil
}

func findAsset(rel *release, name string) *asset {
	for i := range rel.Assets {
		if rel.Assets[i].Name == name {
			return &rel.Assets[i]
		}
	}
	return nil
}

func assetNames(rel *release) string {
	names := make([]string, 0, len(rel.Assets))
	for _, a := range rel.Assets {
		names = append(names, a.Name)
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// fetchChecksums returns name -> sha256. A missing or unreadable checksums file
// downgrades to an unverified download rather than blocking the run.
func fetchChecksums(client *http.Client, rel *release) map[string]string {
	a := findAsset(rel, checksumsAsset)
	if a == nil {
		return nil
	}
	req, err := newRequest(a.URL)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil
	}

	sums := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			sums[filepath.Base(fields[1])] = strings.ToLower(fields[0])
		}
	}
	return sums
}

func download(client *http.Client, url, dest string) error {
	req, err := newRequest(url)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: %s", filepath.Base(url), resp.Status)
	}

	tmp := dest + ".part"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

func fileSum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sizeOf(fi os.FileInfo) int64 {
	if fi == nil {
		return 0
	}
	return fi.Size()
}

func humanSize(b int64) string {
	if b <= 0 {
		return "unknown size"
	}
	return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20))
}
