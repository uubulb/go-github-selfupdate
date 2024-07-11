package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"

	"code.gitea.io/sdk/gitea"
)

// GiteaUpdater contains Gitea client and its context.
type GiteaUpdater struct {
	api       *gitea.Client
	apiCtx    context.Context
	validator Validator
	filters   []*regexp.Regexp
}

// NewGiteaUpdater creates a new updater instance. It initializes Gitea API client.
// If you set your API token to $GITEA_TOKEN, the client will use it.
func NewGiteaUpdater(endpoint string, config Config) (*GiteaUpdater, error) {
	token := config.APIToken
	if token == "" {
		token = os.Getenv("GITEA_TOKEN")
	}
	// if token == "" {
	// 	token, _ = gitconfig.GithubToken()
	// }
	ctx := context.Background()

	filtersRe := make([]*regexp.Regexp, 0, len(config.Filters))
	for _, filter := range config.Filters {
		re, err := regexp.Compile(filter)
		if err != nil {
			return nil, fmt.Errorf("Could not compile regular expression %q for filtering releases: %v", filter, err)
		}
		filtersRe = append(filtersRe, re)
	}

	client, err := gitea.NewClient(endpoint, gitea.SetToken(token))
	if err != nil {
		return nil, errors.New("Failed to create a new Gitea Client")
	}

	return &GiteaUpdater{api: client, apiCtx: ctx, validator: config.Validator, filters: filtersRe}, nil
}

// DefaultGiteaUpdater creates a new updater instance with default configuration.
// It initializes Gitea API client with default API base URL.
// If you set your API token to $GITEA_TOKEN, the client will use it.
func DefaultGiteaUpdater(endpoint string) *GiteaUpdater {
	token := os.Getenv("GITEA_TOKEN")
	// if token == "" {
	// 	token, _ = gitconfig.GithubToken()
	// }
	ctx := context.Background()
	client, _ := gitea.NewClient(endpoint, gitea.SetToken(token))
	return &GiteaUpdater{api: client, apiCtx: ctx}
}
