package cf

import (
	"context"
	"fmt"

	cfclient "github.com/cloudfoundry/go-cfclient/v3/client"
	"github.com/cloudfoundry/go-cfclient/v3/config"
	"github.com/cloudfoundry/go-cfclient/v3/resource"
)

// PushRequest carries the parameters needed to create/push a CF application.
type PushRequest struct {
	// Name is the application name.
	Name string
	// SpaceGUID is the GUID of the target space.
	SpaceGUID string
	// DockerImage, if non-empty, requests a docker lifecycle instead of buildpack.
	DockerImage string
	// Buildpacks is an optional list of buildpack names/URLs.
	Buildpacks []string
	// EnvVars are optional environment variables to set on the app.
	EnvVars map[string]string
}

// TaskRequest carries the parameters needed to run a one-off CF task.
type TaskRequest struct {
	// Command is the shell command to execute in the task container.
	Command string
	// Name is an optional human-readable task name.
	Name string
	// MemoryInMB overrides the default task memory allocation when > 0.
	MemoryInMB int
	// DiskInMB overrides the default task disk allocation when > 0.
	DiskInMB int
}

// CFClient is the interface through which the CF adapter interacts with the
// Cloud Foundry API. All methods follow the real go-cfclient v3 resource types
// so that production code and fakes share the same type vocabulary.
type CFClient interface {
	// GetAppByName returns the first app matching name in the given space.
	GetAppByName(ctx context.Context, name, spaceGUID string) (*resource.App, error)

	// PushApp creates (or re-creates) an application from the supplied request.
	// In v1 this performs a basic app create; a full manifest push can be
	// added later via the operation.AppPushOperation helper.
	PushApp(ctx context.Context, req PushRequest) (*resource.App, error)

	// RunTask starts a one-off task on the specified app.
	RunTask(ctx context.Context, appGUID string, req TaskRequest) (*resource.Task, error)

	// GetTask fetches the current state of a task by its GUID.
	GetTask(ctx context.Context, taskGUID string) (*resource.Task, error)

	// CancelTask requests cancellation of a running task.
	CancelTask(ctx context.Context, taskGUID string) error

	// GetOrgByName returns the first organization matching the given name.
	GetOrgByName(ctx context.Context, name string) (*resource.Organization, error)

	// GetSpaceByName returns the first space matching name inside the given org.
	GetSpaceByName(ctx context.Context, orgGUID, name string) (*resource.Space, error)
}

// realCFClient wraps the official go-cfclient v3 Client and satisfies CFClient.
type realCFClient struct {
	cf *cfclient.Client
}

// NewRealCFClient constructs a CFClient that talks to a real CF API endpoint
// using username/password authentication.
func NewRealCFClient(apiURL, username, password string) (CFClient, error) {
	cfg, err := config.New(apiURL, config.UserPassword(username, password))
	if err != nil {
		return nil, fmt.Errorf("cf config: %w", err)
	}
	cf, err := cfclient.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("cf client: %w", err)
	}
	return &realCFClient{cf: cf}, nil
}

// GetAppByName returns the first app whose name matches in the specified space.
func (c *realCFClient) GetAppByName(ctx context.Context, name, spaceGUID string) (*resource.App, error) {
	opts := cfclient.NewAppListOptions()
	opts.Names.EqualTo(name)
	opts.SpaceGUIDs.EqualTo(spaceGUID)
	app, err := c.cf.Applications.First(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("get app %q in space %q: %w", name, spaceGUID, err)
	}
	return app, nil
}

// PushApp creates an application in CF from the supplied PushRequest.
// A Docker lifecycle is used when DockerImage is set; otherwise buildpack.
func (c *realCFClient) PushApp(ctx context.Context, req PushRequest) (*resource.App, error) {
	spaceRel := resource.SpaceRelationship{
		Space: resource.ToOneRelationship{
			Data: &resource.Relationship{GUID: req.SpaceGUID},
		},
	}
	createReq := &resource.AppCreate{
		Name:          req.Name,
		Relationships: spaceRel,
	}
	if req.EnvVars != nil {
		createReq.EnvironmentVariables = req.EnvVars
	}
	if req.DockerImage != "" {
		createReq.Lifecycle = &resource.Lifecycle{
			Type: "docker",
			Data: resource.DockerLifecycle{Image: req.DockerImage},
		}
	} else if len(req.Buildpacks) > 0 {
		createReq.Lifecycle = &resource.Lifecycle{
			Type: "buildpack",
			Data: resource.BuildpackLifecycle{Buildpacks: req.Buildpacks},
		}
	}
	app, err := c.cf.Applications.Create(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("push app %q: %w", req.Name, err)
	}
	return app, nil
}

// RunTask starts a one-off task on the given app.
func (c *realCFClient) RunTask(ctx context.Context, appGUID string, req TaskRequest) (*resource.Task, error) {
	create := resource.NewTaskCreateWithCommand(req.Command)
	if req.Name != "" {
		create.WithName(req.Name)
	}
	if req.MemoryInMB > 0 {
		create.WithMemoryInMB(req.MemoryInMB)
	}
	if req.DiskInMB > 0 {
		create.WithDiskInMB(req.DiskInMB)
	}
	task, err := c.cf.Tasks.Create(ctx, appGUID, create)
	if err != nil {
		return nil, fmt.Errorf("run task on app %q: %w", appGUID, err)
	}
	return task, nil
}

// GetTask returns the current state of a CF task.
func (c *realCFClient) GetTask(ctx context.Context, taskGUID string) (*resource.Task, error) {
	task, err := c.cf.Tasks.Get(ctx, taskGUID)
	if err != nil {
		return nil, fmt.Errorf("get task %q: %w", taskGUID, err)
	}
	return task, nil
}

// CancelTask requests cancellation of a running task; the returned task is
// discarded because the interface only surfaces an error.
func (c *realCFClient) CancelTask(ctx context.Context, taskGUID string) error {
	_, err := c.cf.Tasks.Cancel(ctx, taskGUID)
	if err != nil {
		return fmt.Errorf("cancel task %q: %w", taskGUID, err)
	}
	return nil
}

// GetOrgByName returns the first org matching name.
func (c *realCFClient) GetOrgByName(ctx context.Context, name string) (*resource.Organization, error) {
	opts := cfclient.NewOrganizationListOptions()
	opts.Names.EqualTo(name)
	org, err := c.cf.Organizations.First(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("get org %q: %w", name, err)
	}
	return org, nil
}

// GetSpaceByName returns the first space matching name inside the given org.
func (c *realCFClient) GetSpaceByName(ctx context.Context, orgGUID, name string) (*resource.Space, error) {
	opts := cfclient.NewSpaceListOptions()
	opts.Names.EqualTo(name)
	opts.OrganizationGUIDs.EqualTo(orgGUID)
	space, err := c.cf.Spaces.First(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("get space %q in org %q: %w", name, orgGUID, err)
	}
	return space, nil
}
