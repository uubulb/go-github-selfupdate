package selfupdate

import (
	"github.com/blang/semver"
	"os"
	"testing"
)

func TestGitHubTokenEnv(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("because $GITHUB_TOKEN is not set")
	}
	_ = NewDetector()
}

func TestDetectReleaseWithVersionPrefix(t *testing.T) {
	v, ok, err := DetectLatest("rhysd/github-clone-all")
	if err != nil {
		t.Fatal("Fetch failed:", err)
	}
	if !ok {
		t.Fatal("Failed to detect latest")
	}
	if v.LE(semver.MustParse("2.0.0")) {
		t.Fatal("Incorrect version:", v)
	}
}

func TestInvalidSlug(t *testing.T) {
	d := NewDetector()

	for _, slug := range []string{
		"foo",
		"/",
		"foo/",
		"/bar",
		"foo/bar/piyo",
	} {
		_, _, err := d.DetectLatest(slug)
		if err == nil {
			t.Error(slug, "should be invalid slug")
		}
	}
}

func TestNonExistingRepo(t *testing.T) {
	d := NewDetector()
	v, ok, err := d.DetectLatest("rhysd/non-existing-repo")
	if err != nil {
		t.Fatal("Non-existing repo should not cause an error:", v)
	}
	if ok {
		t.Fatal("Release for non-existing repo should not be found")
	}
}

func TestNoReleaseFound(t *testing.T) {
	d := NewDetector()
	_, ok, err := d.DetectLatest("rhysd/misc")
	if err != nil {
		t.Fatal("Repo having no release should not cause an error:", err)
	}
	if ok {
		t.Fatal("Repo having no release should not be found")
	}
}