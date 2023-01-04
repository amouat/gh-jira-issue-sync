// Copyright 2017 CoreOS, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package github

import (
	"context"
	"fmt"

	gogh "github.com/google/go-github/v48/github"
	"golang.org/x/oauth2"
	"sigs.k8s.io/release-sdk/github"

	"github.com/uwu-tools/gh-jira-issue-sync/internal/config"
	"github.com/uwu-tools/gh-jira-issue-sync/internal/options"
)

// Client is a wrapper around the GitHub API Client library we
// use. It allows us to swap in other implementations, such as a dry run
// clients, or mock clients for testing.
type Client interface {
	ListIssues() ([]*gogh.Issue, error)
	ListComments(issue *gogh.Issue) ([]*gogh.IssueComment, error)
	GetUser(login string) (*gogh.User, error)
}

// githubClient is a standard GitHub clients, that actually makes all of the
// requests against the GitHub REST API. It is the canonical implementation
// of GitHubClient.
type githubClient struct {
	cfg        *config.Config
	client     *github.GitHub
	goghClient *gogh.Client
}

const itemsPerPage = 100

// ListIssues returns the list of GitHub issues since the last run of the tool.
func (g *githubClient) ListIssues() ([]*gogh.Issue, error) {
	log := g.cfg.GetLogger()

	owner, repo := g.cfg.GetRepo()

	var issues []*gogh.Issue

	// TODO(github): Should issue state be configurable?
	issueState := github.IssueStateAll

	// TODO(github): Consider if these options need to be exposed upstream.
	/*
		gogh.IssueListByRepoOptions{
			Since:     g.cfg.GetSinceParam(),
			State:     string(issueState),
			Sort:      "created",
			Direction: "asc",
			ListOptions: gogh.ListOptions{
				PerPage: itemsPerPage,
			},
		}
	*/
	is, err := g.client.ListIssues(owner, repo, issueState)
	if err != nil {
		return nil, fmt.Errorf("listing GitHub issues: %w", err)
	}

	for _, v := range is {
		// If PullRequestLinks is not nil, it's a Pull Request
		if v.PullRequestLinks == nil {
			issues = append(issues, v)
		}
	}

	log.Debug("Collected all GitHub issues")
	return issues, nil
}

// ListComments returns the list of all comments on a GitHub issue in
// ascending order of creation.
func (g *githubClient) ListComments(issue *gogh.Issue) ([]*gogh.IssueComment, error) {
	log := g.cfg.GetLogger()

	owner, repo := g.cfg.GetRepo()
	issueNum := issue.GetNumber()
	since := g.cfg.GetSinceParam()

	comments, err := g.client.ListComments(
		owner,
		repo,
		issueNum,
		github.SortCreated,
		github.SortDirectionAscending,
		&since,
	)
	if err != nil {
		log.Errorf("Error retrieving GitHub comments for issue #%d. Error: %v.", issueNum, err)
		return nil, fmt.Errorf(
			"listing GitHub comments for issue #%d. Error: %w",
			issueNum,
			err,
		)
	}

	return comments, nil
}

// GetUser returns a GitHub user from its login.
func (g *githubClient) GetUser(login string) (*gogh.User, error) {
	log := g.cfg.GetLogger()

	log.Debugf("Retrieving GitHub user (%s)", login)
	user, resp, err := g.goghClient.Users.Get(context.Background(), login)
	if err != nil {
		return nil, fmt.Errorf(
			"retrieving GitHub user (%s): %w (response: %v)",
			login,
			err,
			resp,
		)
	}

	return user, nil
}

// New creates a GitHubClient and returns it; which
// implementation it uses depends on the configuration of this
// run. For example, a dry-run clients may be created which does
// not make any requests that would change anything on the server,
// but instead simply prints out the actions that it's asked to take.
func New(cfg *config.Config) (Client, error) {
	log := cfg.GetLogger()

	token := cfg.GetConfigString(options.ConfigKeyGitHubToken)
	client, err := github.NewWithToken(token)
	if err != nil {
		return nil, fmt.Errorf("creating sync client: %w", err)
	}

	opts := &github.Options{
		ItemsPerPage: itemsPerPage,
	}

	client.SetOptions(opts)

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: token,
		},
	)
	tc := oauth2.NewClient(ctx, ts)

	goghClient := gogh.NewClient(tc)

	ret := &githubClient{
		cfg:        cfg,
		client:     client,
		goghClient: goghClient,
	}

	log.Debug("Successfully connected to GitHub.")
	return ret, nil
}
