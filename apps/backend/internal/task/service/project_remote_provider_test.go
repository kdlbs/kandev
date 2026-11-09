package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	gitLabProjectSource     = "https://gitlab.example.test:8443/group/subgroup/project"
	gitLabProjectOrigin     = "https://gitlab.example.test:8443"
	pluginProjectSource     = "https://bitbucket.example.test/projects/TEAM/fixture"
	pluginProjectCloneURL   = "https://bitbucket.example.test/scm/TEAM/fixture.git"
	pluginProjectProviderID = "fixture-source-control"
)

func projectSourceReaderFor(workspaceID string, sources ...string) ProjectRepositorySourceReader {
	return projectRepositorySourceReaderFunc(func(context.Context, string) (ProjectRepositorySources, error) {
		return ProjectRepositorySources{WorkspaceID: workspaceID, Sources: sources}, nil
	})
}

func discoveredPluginCandidate(input TaskRepositoryInput) TaskRepositoryInput {
	return TaskRepositoryInput{
		RemoteURL: pluginProjectCloneURL, Provider: pluginProjectProviderID,
		ProviderHost: "https://bitbucket.example.test", ProviderScope: "workspace-a",
		ProviderRepoID: "repo-42", ProviderOwner: "TEAM", ProviderName: "fixture",
		DefaultBranch: "main", BaseBranch: input.BaseBranch, CheckoutBranch: input.CheckoutBranch,
		TrustedProviderDescriptor: true,
	}
}

// exactGitLabOriginVerifier accepts only one configured workspace origin, like
// the production connection reader.
type exactGitLabOriginVerifier struct {
	origin string
	calls  []string
}

func (v *exactGitLabOriginVerifier) VerifyGitLabRepositoryOrigin(_ context.Context, workspaceID, origin string) error {
	v.calls = append(v.calls, workspaceID+"|"+origin)
	if origin != v.origin {
		return NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
	}
	return nil
}

// @covers AC-TASKS-PROJECT-REPOSITORIES-001.1, AC-TASKS-PROJECT-REPOSITORIES-001.2,
// AC-TASKS-PROJECT-REPOSITORIES-001.10, AC-TASKS-PROJECT-REPOSITORIES-001.12
func TestCreateTaskProjectRemoteProviders(t *testing.T) {
	t.Run("self-managed GitLab project source", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		ctx := context.Background()
		createRepositorySelectionWorkspace(t, repo)
		verifier := &gitLabOriginVerifierStub{}
		svc.SetGitLabRepositoryOriginVerifier(verifier)
		svc.SetProjectRepositorySourceReader(projectSourceReaderFor("ws-1", gitLabProjectSource))

		result, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
			Title: "GitLab project source", ProjectID: "project-gitlab",
		})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if len(result.Task.Repositories) != 1 {
			t.Fatalf("task repositories = %+v", result.Task.Repositories)
		}
		stored, err := repo.GetRepository(ctx, result.Task.Repositories[0].RepositoryID)
		if err != nil {
			t.Fatalf("GetRepository: %v", err)
		}
		if stored.Provider != providerGitLab || stored.ProviderHost != gitLabProjectOrigin ||
			stored.ProviderOwner != "group/subgroup" || stored.ProviderName != "project" ||
			stored.RemoteURL != gitLabProjectOrigin+"/group/subgroup/project.git" {
			t.Fatalf("stored repository = %+v, want exact GitLab identity", stored)
		}
		if len(verifier.calls) != 1 || verifier.calls[0] != "ws-1|"+gitLabProjectOrigin {
			t.Fatalf("verification calls = %v", verifier.calls)
		}
		attached, err := repo.ListTaskRepositories(ctx, result.Task.ID)
		if err != nil || len(attached) != 1 || attached[0].RepositoryID != stored.ID {
			t.Fatalf("task attachments = %+v, error = %v", attached, err)
		}
	})

	t.Run("plugin project source without a provider hint", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		ctx := context.Background()
		createRepositorySelectionWorkspace(t, repo)
		resolver := &repositorySelectionResolverStub{discover: func(input TaskRepositoryInput) (RepositorySelectionDiscoveryResult, error) {
			return RepositorySelectionDiscoveryResult{Candidates: []TaskRepositoryInput{discoveredPluginCandidate(input)}}, nil
		}}
		svc.SetRepositorySelectionResolver(resolver)
		svc.SetProjectRepositorySourceReader(projectSourceReaderFor("ws-1", pluginProjectSource))

		result, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
			Title: "Plugin project source", ProjectID: "project-plugin",
		})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if len(resolver.discoveries) != 1 || resolver.discoveries[0].Provider != "" {
			t.Fatalf("discovery calls = %+v, want one unhinted pass", resolver.discoveries)
		}
		if len(resolver.calls) != 0 {
			t.Fatalf("explicit resolution calls = %+v, want discovery only", resolver.calls)
		}
		stored, err := repo.GetRepository(ctx, result.Task.Repositories[0].RepositoryID)
		if err != nil {
			t.Fatalf("GetRepository: %v", err)
		}
		if stored.Provider != pluginProjectProviderID || stored.ProviderHost != "https://bitbucket.example.test" ||
			stored.ProviderScope != "workspace-a" || stored.ProviderRepoID != "repo-42" ||
			stored.ProviderOwner != "TEAM" || stored.ProviderName != "fixture" ||
			stored.RemoteURL != pluginProjectCloneURL || stored.DefaultBranch != "main" {
			t.Fatalf("stored repository = %+v, want the exact discovered descriptor", stored)
		}
	})

	t.Run("mixed built-in and plugin project sources", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		ctx := context.Background()
		createRepositorySelectionWorkspace(t, repo)
		svc.SetGitLabRepositoryOriginVerifier(&exactGitLabOriginVerifier{origin: gitLabProjectOrigin})
		svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{
			discover: func(input TaskRepositoryInput) (RepositorySelectionDiscoveryResult, error) {
				if input.RemoteURL != pluginProjectSource {
					return RepositorySelectionDiscoveryResult{}, nil
				}
				return RepositorySelectionDiscoveryResult{Candidates: []TaskRepositoryInput{discoveredPluginCandidate(input)}}, nil
			},
		})
		svc.SetProjectRepositorySourceReader(projectSourceReaderFor("ws-1", gitLabProjectSource, pluginProjectSource))

		result, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
			Title: "Mixed project sources", ProjectID: "project-mixed",
		})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if len(result.Task.Repositories) != 2 {
			t.Fatalf("task repositories = %+v, want both sources attached", result.Task.Repositories)
		}
		providers := []string{}
		for _, attached := range result.Task.Repositories {
			stored, err := repo.GetRepository(ctx, attached.RepositoryID)
			if err != nil {
				t.Fatalf("GetRepository: %v", err)
			}
			providers = append(providers, stored.Provider)
		}
		if providers[0] != providerGitLab || providers[1] != pluginProjectProviderID {
			t.Fatalf("providers = %v, want source order preserved", providers)
		}
	})

	t.Run("a registered GitLab repository is reused by identity", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		ctx := context.Background()
		createRepositorySelectionWorkspace(t, repo)
		svc.SetGitLabRepositoryOriginVerifier(&gitLabOriginVerifierStub{})
		registered, created, err := svc.RegisterRemoteRepository(ctx, &RegisterRemoteRepositoryRequest{
			WorkspaceID: "ws-1", RemoteURL: gitLabProjectSource, DefaultBranch: "dev",
		})
		if err != nil || !created {
			t.Fatalf("RegisterRemoteRepository created=%v err=%v", created, err)
		}
		svc.SetProjectRepositorySourceReader(projectSourceReaderFor("ws-1", gitLabProjectSource))
		result, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
			Title: "Reuse registered GitLab", ProjectID: "project-gitlab-registered",
		})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if result.Task.Repositories[0].RepositoryID != registered.ID {
			t.Fatalf("repository = %q, want the registered row %q", result.Task.Repositories[0].RepositoryID, registered.ID)
		}
		repositories, err := repo.ListRepositories(ctx, "ws-1")
		if err != nil || len(repositories) != 1 {
			t.Fatalf("repositories = %+v, error = %v; want no duplicate row", repositories, err)
		}
	})
}

// @covers AC-TASKS-PROJECT-REPOSITORIES-001.6, AC-TASKS-PROJECT-REPOSITORIES-001.11,
// AC-TASKS-PROJECT-REPOSITORIES-001.13
func TestProjectRemoteProviderFailuresBeforeWrites(t *testing.T) {
	tests := []struct {
		name    string
		sources []string
		setup   func(*Service)
		want    RepositorySelectionErrorCode
	}{
		{
			name:    "two verified built-in providers are ambiguous",
			sources: []string{gitLabProjectSource},
			setup: func(svc *Service) {
				svc.SetGitHubRepositoryOriginVerifier(&enterpriseRepositoryOriginStub{origin: gitLabProjectOrigin})
				svc.SetGitLabRepositoryOriginVerifier(&gitLabOriginVerifierStub{})
			},
			want: RepositorySelectionErrorInvalid,
		},
		{
			name:    "unknown host with no candidate",
			sources: []string{"https://unknown.example.test/team/repo"},
			want:    RepositorySelectionErrorInvalid,
		},
		{
			name:    "foreign workspace GitLab connection",
			sources: []string{gitLabProjectSource},
			setup: func(svc *Service) {
				svc.SetGitLabRepositoryOriginVerifier(&gitLabOriginVerifierStub{
					err: NewRepositorySelectionError(RepositorySelectionErrorInvalid, errors.New("origin mismatch")),
				})
			},
			want: RepositorySelectionErrorInvalid,
		},
		{
			name:    "disconnected plugin discovery is unavailable",
			sources: []string{pluginProjectSource},
			setup: func(svc *Service) {
				svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{
					discover: func(TaskRepositoryInput) (RepositorySelectionDiscoveryResult, error) {
						return RepositorySelectionDiscoveryResult{}, context.DeadlineExceeded
					},
				})
			},
			want: RepositorySelectionErrorUnavailable,
		},
		{
			name:    "cancelled discovery publishes nothing",
			sources: []string{pluginProjectSource},
			setup: func(svc *Service) {
				svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{
					discover: func(TaskRepositoryInput) (RepositorySelectionDiscoveryResult, error) {
						return RepositorySelectionDiscoveryResult{}, context.Canceled
					},
				})
			},
			want: RepositorySelectionErrorUnavailable,
		},
		{
			name:    "invalid discovered clone origin contributes no candidate",
			sources: []string{pluginProjectSource},
			setup: func(svc *Service) {
				svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{
					discover: func(input TaskRepositoryInput) (RepositorySelectionDiscoveryResult, error) {
						candidate := discoveredPluginCandidate(input)
						candidate.RemoteURL = "https://attacker.example.test/team/repo.git"
						return RepositorySelectionDiscoveryResult{Candidates: []TaskRepositoryInput{candidate}}, nil
					},
				})
			},
			want: RepositorySelectionErrorInvalid,
		},
		{
			name:    "a rejected source after a valid one fails the list",
			sources: []string{pluginProjectSource, "https://unknown.example.test/team/repo"},
			setup: func(svc *Service) {
				svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{
					discover: func(input TaskRepositoryInput) (RepositorySelectionDiscoveryResult, error) {
						if input.RemoteURL != pluginProjectSource {
							return RepositorySelectionDiscoveryResult{}, nil
						}
						return RepositorySelectionDiscoveryResult{Candidates: []TaskRepositoryInput{discoveredPluginCandidate(input)}}, nil
					},
				})
			},
			want: RepositorySelectionErrorInvalid,
		},
		{
			name:    "a rejected source before a valid one fails the list",
			sources: []string{"https://unknown.example.test/team/repo", pluginProjectSource},
			setup: func(svc *Service) {
				svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{
					discover: func(input TaskRepositoryInput) (RepositorySelectionDiscoveryResult, error) {
						if input.RemoteURL != pluginProjectSource {
							return RepositorySelectionDiscoveryResult{}, nil
						}
						return RepositorySelectionDiscoveryResult{Candidates: []TaskRepositoryInput{discoveredPluginCandidate(input)}}, nil
					},
				})
			},
			want: RepositorySelectionErrorInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, bus, repo := createTestService(t)
			ctx := context.Background()
			createRepositorySelectionWorkspace(t, repo)
			if test.setup != nil {
				test.setup(svc)
			}
			svc.SetProjectRepositorySourceReader(projectSourceReaderFor("ws-1", test.sources...))

			_, err := svc.CreateTask(ctx, &CreateTaskRequest{
				WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
				Title: "Rejected project source", ProjectID: "project-rejected",
			})
			assertRepositorySelectionError(t, err, test.want, "")

			repositories, listErr := repo.ListRepositories(ctx, "ws-1")
			if listErr != nil || len(repositories) != 0 {
				t.Fatalf("repositories = %+v, error = %v; want no writes", repositories, listErr)
			}
			tasks, listErr := repo.ListTasks(ctx, "wf-1")
			if listErr != nil || len(tasks) != 0 {
				t.Fatalf("tasks = %+v, error = %v; want no writes", tasks, listErr)
			}
			for _, event := range bus.GetPublishedEvents() {
				if event.Type == "task.created" {
					t.Fatalf("task.created published for a rejected source")
				}
			}
		})
	}
}

// @covers AC-TASKS-PROJECT-REPOSITORIES-001.12
func TestCreateTaskProjectRemoteProviderAcceptsUniqueMatchBesideCandidateFailure(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createRepositorySelectionWorkspace(t, repo)
	svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{
		discover: func(input TaskRepositoryInput) (RepositorySelectionDiscoveryResult, error) {
			return RepositorySelectionDiscoveryResult{
				Candidates: []TaskRepositoryInput{discoveredPluginCandidate(input)},
				Failure:    NewRepositorySelectionFailure(RepositorySelectionErrorUnavailable, nil),
			}, nil
		},
	})
	svc.SetProjectRepositorySourceReader(projectSourceReaderFor("ws-1", pluginProjectSource))

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		Title: "Unique match beside failure", ProjectID: "project-unique-match",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if len(result.Task.Repositories) != 1 {
		t.Fatalf("task repositories = %+v", result.Task.Repositories)
	}
	stored, err := repo.GetRepository(ctx, result.Task.Repositories[0].RepositoryID)
	if err != nil || stored.Provider != pluginProjectProviderID {
		t.Fatalf("stored repository = %+v, error = %v", stored, err)
	}
}

// @covers AC-TASKS-PROJECT-REPOSITORIES-001.13
func TestProjectRemoteProviderKeepsExplicitSelectionsPinned(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createRepositorySelectionWorkspace(t, repo)
	resolver := &repositorySelectionResolverStub{resolve: authoritativeRepositoryInput}
	svc.SetRepositorySelectionResolver(resolver)

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		Title: "Explicit plugin selection",
		Repositories: []TaskRepositoryInput{{
			RemoteURL: pluginProjectSource, Provider: pluginProjectProviderID, BaseBranch: "main",
		}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if len(resolver.discoveries) != 0 {
		t.Fatalf("discovery calls = %+v, want explicit selections pinned to their provider", resolver.discoveries)
	}
	if len(resolver.calls) != 1 {
		t.Fatalf("explicit resolution calls = %d, want one", len(resolver.calls))
	}
}

// @covers AC-TASKS-PROJECT-REPOSITORIES-001.1, AC-TASKS-PROJECT-REPOSITORIES-001.10,
// AC-TASKS-PROJECT-REPOSITORIES-001.13
func TestCreateTaskSavedRepositoryIDSkipsRemoteProviderAdmission(t *testing.T) {
	t.Run("plugin-backed saved repository with unavailable provider", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		ctx := context.Background()
		createRepositorySelectionWorkspace(t, repo)
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: "repo-plugin-saved", WorkspaceID: "ws-1", Name: "fixture", SourceType: "remote",
			Provider: pluginProjectProviderID, ProviderHost: "https://bitbucket.example.test",
			ProviderRepoID: "repo-42", ProviderOwner: "TEAM", ProviderName: "fixture",
			RemoteURL: pluginProjectCloneURL, DefaultBranch: "main",
		}); err != nil {
			t.Fatalf("CreateRepository: %v", err)
		}

		result, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Saved plugin repository",
			Repositories: []TaskRepositoryInput{{
				RepositoryID: "repo-plugin-saved", RemoteURL: pluginProjectSource,
				Provider: pluginProjectProviderID, ProviderHost: "https://bitbucket.example.test",
				BaseBranch: "main",
			}},
		})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if len(result.Task.Repositories) != 1 || result.Task.Repositories[0].RepositoryID != "repo-plugin-saved" {
			t.Fatalf("task repositories = %+v, want the saved repository ID", result.Task.Repositories)
		}
	})

	t.Run("self-managed GitLab saved repository without a connection", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		ctx := context.Background()
		createRepositorySelectionWorkspace(t, repo)
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: "repo-gitlab-saved", WorkspaceID: "ws-1", Name: "project", SourceType: "remote",
			Provider: providerGitLab, ProviderHost: gitLabProjectOrigin,
			ProviderOwner: "group/subgroup", ProviderName: "project",
			RemoteURL: gitLabProjectOrigin + "/group/subgroup/project.git", DefaultBranch: "main",
		}); err != nil {
			t.Fatalf("CreateRepository: %v", err)
		}

		result, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Saved GitLab repository",
			Repositories: []TaskRepositoryInput{{
				RepositoryID: "repo-gitlab-saved", RemoteURL: gitLabProjectSource,
				Provider: providerGitLab, ProviderHost: gitLabProjectOrigin, BaseBranch: "main",
			}},
		})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if len(result.Task.Repositories) != 1 || result.Task.Repositories[0].RepositoryID != "repo-gitlab-saved" {
			t.Fatalf("task repositories = %+v, want the saved repository ID", result.Task.Repositories)
		}
	})

	t.Run("foreign workspace saved repository is still refused", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		ctx := context.Background()
		createRepositorySelectionWorkspace(t, repo)
		if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Other"}); err != nil {
			t.Fatalf("CreateWorkspace: %v", err)
		}
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: "repo-foreign", WorkspaceID: "ws-2", Name: "fixture", SourceType: "remote",
			Provider: pluginProjectProviderID, ProviderHost: "https://bitbucket.example.test",
			ProviderRepoID: "repo-42", ProviderOwner: "TEAM", ProviderName: "fixture",
			RemoteURL: pluginProjectCloneURL, DefaultBranch: "main",
		}); err != nil {
			t.Fatalf("CreateRepository: %v", err)
		}

		_, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Foreign saved repository",
			Repositories: []TaskRepositoryInput{{
				RepositoryID: "repo-foreign", RemoteURL: pluginProjectSource,
				Provider: pluginProjectProviderID, BaseBranch: "main",
			}},
		})
		if !errors.Is(err, ErrTaskReferenceNotFound) {
			t.Fatalf("error = %v, want ErrTaskReferenceNotFound", err)
		}
	})
}
