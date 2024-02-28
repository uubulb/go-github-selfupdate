package selfupdate

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/blang/semver"
	"github.com/inconshreveable/go-update"
)

func uncompressAndUpdate(src io.Reader, assetURL, cmdPath string) error {
	_, cmd := filepath.Split(cmdPath)
	asset, err := UncompressCommand(src, assetURL, cmd)
	if err != nil {
		return err
	}

	log.Println("Will update", cmdPath, "to the latest downloaded from", assetURL)
	return update.Apply(asset, update.Options{
		TargetPath: cmdPath,
	})
}

func (up *Updater) downloadDirectlyFromURL(assetURL string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", assetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create HTTP request to %s: %s", assetURL, err)
	}

	req.Header.Add("Accept", "application/octet-stream")
	req = req.WithContext(up.apiCtx)

	// OAuth HTTP client is not available to download blob from URL when the URL is a redirect URL
	// returned from GitHub Releases API (response status 400).
	// Use default HTTP client instead.
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Failed to download a release file from %s: %s", assetURL, err)
	}

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("Failed to download a release file from %s: Not successful status %d", assetURL, res.StatusCode)
	}

	return res.Body, nil
}

func useGitHubMirrorIfIpInChina(rel *Release) {
	resp, err := http.Get("https://api.myip.la/json")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	if !strings.Contains("China", string(body)) {
		return
	}
	resp, err = http.Get("https://dn-dao-github-mirror.daocloud.io")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	if !strings.Contains("GitHub", string(body)) {
		return
	}
	log.Println("China IP detected, using kkgithub mirror")
	rel.AssetURL = strings.Replace(rel.AssetURL, "github.com", "dn-dao-github-mirror.daocloud.io", 1)
}

// UpdateTo downloads an executable from GitHub Releases API and replace current binary with the downloaded one.
// It downloads a release asset via GitHub Releases API so this function is available for update releases on private repository.
// If a redirect occurs, it fallbacks into directly downloading from the redirect URL.
func (up *Updater) UpdateTo(rel *Release, cmdPath string) error {
	useGitHubMirrorIfIpInChina(rel)
	src, err := up.downloadDirectlyFromURL(rel.AssetURL)
	if err != nil {
		return err
	}
	defer src.Close()
	data, err := io.ReadAll(src)
	if err != nil {
		return fmt.Errorf("Failed reading validation asset body: %v", err)
	}
	return uncompressAndUpdate(bytes.NewReader(data), rel.AssetURL, cmdPath)
}

// UpdateCommand updates a given command binary to the latest version.
// 'slug' represents 'owner/name' repository on GitHub and 'current' means the current version.
func (up *Updater) UpdateCommand(cmdPath string, current semver.Version, slug string) (*Release, error) {
	if runtime.GOOS == "windows" && !strings.HasSuffix(cmdPath, ".exe") {
		// Ensure to add '.exe' to given path on Windows
		cmdPath = cmdPath + ".exe"
	}

	stat, err := os.Lstat(cmdPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to stat '%s'. File may not exist: %s", cmdPath, err)
	}
	if stat.Mode()&os.ModeSymlink != 0 {
		p, err := filepath.EvalSymlinks(cmdPath)
		if err != nil {
			return nil, fmt.Errorf("Failed to resolve symlink '%s' for executable: %s", cmdPath, err)
		}
		cmdPath = p
	}

	rel, ok, err := up.DetectLatest(slug)
	if err != nil {
		return nil, err
	}
	if !ok {
		log.Println("No release detected. Current version is considered up-to-date")
		return &Release{Version: current}, nil
	}
	if current.Equals(rel.Version) {
		log.Println("Current version", current, "is the latest. Update is not needed")
		return rel, nil
	}
	log.Println("Will update", cmdPath, "to the latest version", rel.Version)
	if err := up.UpdateTo(rel, cmdPath); err != nil {
		return nil, err
	}
	return rel, nil
}

// UpdateSelf updates the running executable itself to the latest version.
// 'slug' represents 'owner/name' repository on GitHub and 'current' means the current version.
func (up *Updater) UpdateSelf(current semver.Version, slug string) (*Release, error) {
	cmdPath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return up.UpdateCommand(cmdPath, current, slug)
}

// UpdateTo downloads an executable from assetURL and replace the current binary with the downloaded one.
// This function is low-level API to update the binary. Because it does not use GitHub API and downloads asset directly from the URL via HTTP,
// this function is not available to update a release for private repositories.
// cmdPath is a file path to command executable.
func UpdateTo(assetURL, cmdPath string) error {
	up := DefaultUpdater()
	src, err := up.downloadDirectlyFromURL(assetURL)
	if err != nil {
		return err
	}
	defer src.Close()
	return uncompressAndUpdate(src, assetURL, cmdPath)
}

// UpdateCommand updates a given command binary to the latest version.
// This function is a shortcut version of updater.UpdateCommand.
func UpdateCommand(cmdPath string, current semver.Version, slug string) (*Release, error) {
	return DefaultUpdater().UpdateCommand(cmdPath, current, slug)
}

// UpdateSelf updates the running executable itself to the latest version.
// This function is a shortcut version of updater.UpdateSelf.
func UpdateSelf(current semver.Version, slug string) (*Release, error) {
	return DefaultUpdater().UpdateSelf(current, slug)
}
