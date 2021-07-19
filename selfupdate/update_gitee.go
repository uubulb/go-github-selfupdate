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
)

func (up *GiteeUpdater) downloadDirectlyFromURL(assetURL string) (io.ReadCloser, error) {
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

// UpdateTo downloads an executable from GitHub Releases API and replace current binary with the downloaded one.
// It downloads a release asset via GitHub Releases API so this function is available for update releases on private repository.
// If a redirect occurs, it fallbacks into directly downloading from the redirect URL.
func (up *GiteeUpdater) UpdateTo(rel *Release, cmdPath string) error {
	src, err := up.downloadDirectlyFromURL(rel.AssetURL)
	if err != nil {
		return err
	}
	defer src.Close()

	data, err := io.ReadAll(src)
	if err != nil {
		return fmt.Errorf("Failed reading validation asset body: %v", err)
	}

	return uncompressAndUpdate(bytes.NewReader(data), rel.AssetURL, cmdPath, up.binaryName)
}

// UpdateCommand updates a given command binary to the latest version.
// 'slug' represents 'owner/name' repository on GitHub and 'current' means the current version.
func (up *GiteeUpdater) UpdateCommand(cmdPath string, current semver.Version, slug string) (*Release, error) {
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
// 'slug' represents 'owner/name' repository on Gitee and 'current' means the current version.
func (up *GiteeUpdater) UpdateSelf(current semver.Version, slug string) (*Release, error) {
	cmdPath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return up.UpdateCommand(cmdPath, current, slug)
}

// UpdateToGitee downloads an executable from assetURL and replace the current binary with the downloaded one.
// This function is low-level API to update the binary. Because it does not use Gitee API and downloads asset directly from the URL via HTTP,
// this function is not available to update a release for private repositories.
// cmdPath is a file path to command executable.
func UpdateToGitee(assetURL, cmdPath string) error {
	up := DefaultGiteeUpdater()
	src, err := up.downloadDirectlyFromURL(assetURL)
	if err != nil {
		return err
	}
	defer src.Close()
	return uncompressAndUpdate(src, assetURL, cmdPath, up.binaryName)
}

// UpdateCommandGitee updates a given command binary to the latest version.
// This function is a shortcut version of updater.UpdateCommand.
func UpdateCommandGitee(cmdPath string, current semver.Version, slug string) (*Release, error) {
	return DefaultGiteeUpdater().UpdateCommand(cmdPath, current, slug)
}

// UpdateSelfGitee updates the running executable itself to the latest version.
// This function is a shortcut version of updater.UpdateSelf.
func UpdateSelfGitee(current semver.Version, slug string) (*Release, error) {
	return DefaultGiteeUpdater().UpdateSelf(current, slug)
}
