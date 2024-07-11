package selfupdate

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"gitee.com/naibahq/go-gitee/gitee"
	"github.com/blang/semver"
)

func findAssetFromReleaseGitee(rel gitee.Release,
	suffixes []string, targetVersion string, filters []*regexp.Regexp) (*gitee.Asset, semver.Version, bool) {

	if targetVersion != "" && targetVersion != rel.TagName {
		log.Println("Skip", rel.TagName, "not matching to specified version", targetVersion)
		return nil, semver.Version{}, false
	}

	if targetVersion == "" && rel.Prerelease {
		log.Println("Skip pre-release version", rel.TagName)
		return nil, semver.Version{}, false
	}

	verText := rel.TagName
	indices := reVersion.FindStringIndex(verText)
	if indices == nil {
		log.Println("Skip version not adopting semver", verText)
		return nil, semver.Version{}, false
	}
	if indices[0] > 0 {
		log.Println("Strip prefix of version", verText[:indices[0]], "from", verText)
		verText = verText[indices[0]:]
	}

	// If semver cannot parse the version text, it means that the text is not adopting
	// the semantic versioning. So it should be skipped.
	ver, err := semver.Make(verText)
	if err != nil {
		log.Println("Failed to parse a semantic version", verText)
		return nil, semver.Version{}, false
	}

	for _, asset := range rel.Assets {
		name := asset.Name
		if len(filters) > 0 {
			// if some filters are defined, match them: if any one matches, the asset is selected
			matched := false
			for _, filter := range filters {
				if filter.MatchString(name) {
					log.Println("Selected filtered asset", name)
					matched = true
					break
				}
				log.Printf("Skipping asset %q not matching filter %v\n", name, filter)
			}
			if !matched {
				continue
			}
		}

		for _, s := range suffixes {
			if strings.HasSuffix(name, s) { // require version, arch etc
				// default: assume single artifact
				return &asset, ver, true
			}
		}
	}

	log.Println("No suitable asset was found in release", rel.TagName)
	return nil, semver.Version{}, false
}

func findValidationAssetGitee(rel *gitee.Release, validationName string) (*gitee.Asset, bool) {
	for _, asset := range rel.Assets {
		if asset.Name == validationName {
			return &asset, true
		}
	}
	return nil, false
}

func findReleaseAndAssetGitee(rels []gitee.Release,
	targetVersion string,
	filters []*regexp.Regexp) (*gitee.Release, *gitee.Asset, semver.Version, bool) {
	// Generate candidates
	suffixes := make([]string, 0, 2*7*2)
	for _, sep := range []rune{'_', '-'} {
		for _, ext := range []string{".zip", ".tar.gz", ".tgz", ".gzip", ".gz", ".tar.xz", ".xz", ""} {
			suffix := fmt.Sprintf("%s%c%s%s", runtime.GOOS, sep, runtime.GOARCH, ext)
			suffixes = append(suffixes, suffix)
			if runtime.GOOS == "windows" {
				suffix = fmt.Sprintf("%s%c%s.exe%s", runtime.GOOS, sep, runtime.GOARCH, ext)
				suffixes = append(suffixes, suffix)
			}
		}
	}

	var ver semver.Version
	var asset *gitee.Asset
	var release *gitee.Release

	// Find the latest version from the list of releases.
	// Returned list from GitHub API is in the order of the date when created.
	//   ref: https://github.com/nezhahq/go-github-selfupdate/issues/11
	for _, rel := range rels {
		if a, v, ok := findAssetFromReleaseGitee(rel, suffixes, targetVersion, filters); ok {
			// Note: any version with suffix is less than any version without suffix.
			// e.g. 0.0.1 > 0.0.1-beta
			if release == nil || v.GTE(ver) {
				ver = v
				asset = a
				release = &rel
			}
		}
	}

	if release == nil {
		log.Println("Could not find any release for", runtime.GOOS, "and", runtime.GOARCH)
		return nil, nil, semver.Version{}, false
	}

	return release, asset, ver, true
}

// DetectLatest tries to get the latest version of the repository on GitHub. 'slug' means 'owner/name' formatted string.
// It fetches releases information from GitHub API and find out the latest release with matching the tag names and asset names.
// Drafts and pre-releases are ignored. Assets would be suffixed by the OS name and the arch name such as 'foo_linux_amd64'
// where 'foo' is a command name. '-' can also be used as a separator. File can be compressed with zip, gzip, zxip, tar&zip or tar&zxip.
// So the asset can have a file extension for the corresponding compression format such as '.zip'.
// On Windows, '.exe' also can be contained such as 'foo_windows_amd64.exe.zip'.
func (up *GiteeUpdater) DetectLatest(slug string) (release *Release, found bool, err error) {
	return up.DetectVersion(slug, "")
}

// DetectVersion tries to get the given version of the repository on Github. `slug` means `owner/name` formatted string.
// And version indicates the required version.
func (up *GiteeUpdater) DetectVersion(slug string, version string) (release *Release, found bool, err error) {
	repo := strings.Split(slug, "/")
	if len(repo) != 2 || repo[0] == "" || repo[1] == "" {
		return nil, false, fmt.Errorf("Invalid slug format. It should be 'owner/name': %s", slug)
	}

	rels, res, err := up.api.RepositoriesApi.GetV5ReposOwnerRepoReleases(context.Background(), repo[0], repo[1], &gitee.GetV5ReposOwnerRepoReleasesOpts{})
	if err != nil {
		log.Println("API returned an error response:", err)
		if res != nil && res.StatusCode == 404 {
			// 404 means repository not found or release not found. It's not an error here.
			err = nil
			log.Println("API returned 404. Repository or release not found")
		}
		return nil, false, err
	}

	rel, asset, ver, found := findReleaseAndAssetGitee(rels, version, up.filters)
	if !found {
		return nil, false, nil
	}

	url := asset.BrowserDownloadUrl
	log.Println("Successfully fetched the latest release. tag:", rel.TagName, ", name:", rel.Name, ", Asset:", url)

	publishedAt := rel.CreatedAt
	release = &Release{
		ver,
		url,
		-1,
		-1,
		-1,
		"",
		rel.Body,
		rel.Name,
		&publishedAt,
		repo[0],
		repo[1],
	}

	if up.validator != nil {
		validationName := asset.Name + up.validator.Suffix()
		_, ok := findValidationAssetGitee(rel, validationName)
		if !ok {
			return nil, false, fmt.Errorf("Failed finding validation file %q", validationName)
		}
	}

	return release, true, nil
}

// DetectLatestGitee detects the latest release of the slug (owner/repo).
// This function is a shortcut version of updater.DetectLatest() method.
func DetectLatestGitee(slug string) (*Release, bool, error) {
	return DefaultGiteeUpdater().DetectLatest(slug)
}

// DetectVersionGitee detects the given release of the slug (owner/repo) from its version.
func DetectVersionGitee(slug string, version string) (*Release, bool, error) {
	return DefaultGiteeUpdater().DetectVersion(slug, version)
}
