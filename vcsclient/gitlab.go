package vcsclient

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	"github.com/jfrog/froggit-go/vcsutils"
	"github.com/xanzy/go-gitlab"
)

type GitLabClient struct {
	glClient *gitlab.Client
}

func NewGitLabClient(vcsInfo *VcsInfo) (*GitLabClient, error) {
	var client *gitlab.Client
	var err error
	if vcsInfo.ApiEndpoint != "" {
		client, err = gitlab.NewClient(vcsInfo.Token, gitlab.WithBaseURL(vcsInfo.ApiEndpoint))
	} else {
		client, err = gitlab.NewClient(vcsInfo.Token)
	}
	if err != nil {
		return nil, err
	}

	return &GitLabClient{
		glClient: client,
	}, nil
}

func (client *GitLabClient) TestConnection(ctx context.Context) error {
	_, _, err := client.glClient.Projects.ListProjects(nil, gitlab.WithContext(ctx))
	return err
}

func (client *GitLabClient) ListRepositories(ctx context.Context) (map[string][]string, error) {
	results := make(map[string][]string)
	groups, _, err := client.glClient.Groups.ListGroups(nil, gitlab.WithContext(ctx))
	if err != nil {
		return results, err
	}
	for _, group := range groups {
		for pageId := 1; ; pageId++ {
			options := &gitlab.ListGroupProjectsOptions{ListOptions: gitlab.ListOptions{Page: pageId}}
			projects, response, err := client.glClient.Groups.ListGroupProjects(group.Path, options,
				gitlab.WithContext(ctx))
			if err != nil {
				return nil, err
			}

			for _, project := range projects {
				results[group.Path] = append(results[group.Path], project.Path)
			}
			if pageId >= response.TotalPages {
				break
			}
		}

	}
	return results, nil
}

func (client *GitLabClient) ListBranches(ctx context.Context, owner, repository string) ([]string, error) {
	branches, _, err := client.glClient.Branches.ListBranches(getProjectId(owner, repository), nil,
		gitlab.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	results := make([]string, 0, len(branches))
	for _, branch := range branches {
		results = append(results, branch.Name)
	}
	return results, nil
}

func (client *GitLabClient) CreateWebhook(ctx context.Context, owner, repository, branch, payloadUrl string,
	webhookEvents ...vcsutils.WebhookEvent) (string, string, error) {
	token := vcsutils.CreateToken()
	projectHook := createProjectHook(branch, payloadUrl, webhookEvents...)
	options := &gitlab.AddProjectHookOptions{
		Token:                  &token,
		URL:                    &projectHook.URL,
		MergeRequestsEvents:    &projectHook.MergeRequestsEvents,
		PushEvents:             &projectHook.PushEvents,
		PushEventsBranchFilter: &projectHook.PushEventsBranchFilter,
	}
	response, _, err := client.glClient.Projects.AddProjectHook(getProjectId(owner, repository), options,
		gitlab.WithContext(ctx))
	if err != nil {
		return "", "", err
	}
	return strconv.Itoa(response.ID), token, nil
}

func (client *GitLabClient) UpdateWebhook(ctx context.Context, owner, repository, branch, payloadUrl, token,
	webhookId string, webhookEvents ...vcsutils.WebhookEvent) error {
	projectHook := createProjectHook(branch, payloadUrl, webhookEvents...)
	options := &gitlab.EditProjectHookOptions{
		Token:                  &token,
		URL:                    &projectHook.URL,
		MergeRequestsEvents:    &projectHook.MergeRequestsEvents,
		PushEvents:             &projectHook.PushEvents,
		PushEventsBranchFilter: &projectHook.PushEventsBranchFilter,
	}
	intWebhook, err := strconv.Atoi(webhookId)
	if err != nil {
		return err
	}
	_, _, err = client.glClient.Projects.EditProjectHook(getProjectId(owner, repository), intWebhook, options,
		gitlab.WithContext(ctx))
	return err
}

func (client *GitLabClient) DeleteWebhook(ctx context.Context, owner, repository, webhookId string) error {
	intWebhook, err := strconv.Atoi(webhookId)
	if err != nil {
		return err
	}
	_, err = client.glClient.Projects.DeleteProjectHook(getProjectId(owner, repository), intWebhook,
		gitlab.WithContext(ctx))
	return err
}

func (client *GitLabClient) SetCommitStatus(ctx context.Context, commitStatus CommitStatus, owner, repository, ref,
	title, description, detailsUrl string) error {
	options := &gitlab.SetCommitStatusOptions{
		State:       gitlab.BuildStateValue(getGitLabCommitState(commitStatus)),
		Ref:         &ref,
		Name:        &title,
		Description: &description,
		TargetURL:   &detailsUrl,
	}
	_, _, err := client.glClient.Commits.SetCommitStatus(getProjectId(owner, repository), ref, options,
		gitlab.WithContext(ctx))
	return err
}

func (client *GitLabClient) DownloadRepository(ctx context.Context, owner, repository, branch, localPath string) error {
	format := "tar.gz"
	options := &gitlab.ArchiveOptions{
		Format: &format,
		SHA:    &branch,
	}
	response, _, err := client.glClient.Repositories.Archive(getProjectId(owner, repository), options,
		gitlab.WithContext(ctx))
	if err != nil {
		return err
	}
	return vcsutils.Untar(localPath, bytes.NewReader(response), true)
}

func (client *GitLabClient) CreatePullRequest(ctx context.Context, owner, repository, sourceBranch, targetBranch,
	title, description string) error {
	options := &gitlab.CreateMergeRequestOptions{
		Title:        &title,
		Description:  &description,
		SourceBranch: &sourceBranch,
		TargetBranch: &targetBranch,
	}
	_, _, err := client.glClient.MergeRequests.CreateMergeRequest(getProjectId(owner, repository), options,
		gitlab.WithContext(ctx))
	return err
}

func getProjectId(owner, project string) string {
	return fmt.Sprintf("%s/%s", owner, project)
}

func createProjectHook(branch string, payloadUrl string, webhookEvents ...vcsutils.WebhookEvent) *gitlab.ProjectHook {
	options := &gitlab.ProjectHook{URL: payloadUrl}
	for _, webhookEvent := range webhookEvents {
		switch webhookEvent {
		case vcsutils.PrCreated, vcsutils.PrEdited:
			options.MergeRequestsEvents = true
		case vcsutils.Push:
			options.PushEvents = true
			options.PushEventsBranchFilter = branch
		}
	}
	return options
}

func getGitLabCommitState(commitState CommitStatus) string {
	switch commitState {
	case Pass:
		return "success"
	case Fail:
		return "failed"
	case Error:
		return "failed"
	case InProgress:
		return "running"
	}
	return ""
}