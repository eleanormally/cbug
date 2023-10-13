package main

import (
	"context"
	"errors"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

type Docker struct {
	// docker client
	client *client.Client

	activeContainer string
}

type GenerateContainerOpts struct {
	platform string
	image    string
	name     string
}

func NewDocker() (*Docker, error) {
	dockerCli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	return &Docker{
		client: dockerCli,
	}, nil
}

// attempts to find a container with the given name. If opts are provided, then it will attempt to generate a container with the given name if not found
func (d *Docker) setActiveContainer(name string, opts *GenerateContainerOpts) error {
	containers, err := d.client.ContainerList(context.Background(), types.ContainerListOptions{
		All: true,
	})
	if err != nil {
		return err
	}

	for _, c := range containers {
		if c.Names[0] == "/"+name {
			d.activeContainer = c.ID
			return nil
		}
	}

	if opts == nil {
		return errors.New("Container not found")
	}
	return d.generateContainer(opts)

}

func (d *Docker) removeActiveContainer(container *types.Container) error {
	return d.client.ContainerRemove(context.Background(), container.ID, types.ContainerRemoveOptions{
		Force: true,
	})
}

func (d *Docker) findImage(image string) (*types.ImageSummary, error) {
	images, err := d.client.ImageList(context.Background(), types.ImageListOptions{
		All: true,
	})
	if err != nil {
		return nil, err
	}

	for _, i := range images {
		for _, tag := range i.RepoTags {
			if tag == image {
				return &i, nil
			}
		}
	}

	return nil, errors.New("Image not found")
}

func (d *Docker) generateContainer(opts *GenerateContainerOpts) error {

	_, err := d.findImage(opts.image)
	if err != nil {
		return err
	}

	contInfo, err := d.client.ContainerCreate(
		context.Background(),
		&container.Config{
			Hostname:        "",
			Domainname:      "",
			User:            "Cbug",
			AttachStdin:     true,
			AttachStdout:    true,
			AttachStderr:    true,
			ExposedPorts:    nil,
			Tty:             true,
			OpenStdin:       true,
			StdinOnce:       true,
			Env:             nil,
			Healthcheck:     &container.HealthConfig{},
			ArgsEscaped:     false,
			Image:           opts.image,
			Volumes:         nil,
			WorkingDir:      "/debugger",
			NetworkDisabled: false,
			StopSignal:      "SIGTERM",
			StopTimeout:     new(int),
			Shell:           []string{"/bin/bash"},
		},
		&container.HostConfig{},
		nil,
		&specs.Platform{
			Architecture: opts.platform,
			OS:           "linux",
		},
		opts.name,
	)
	if err != nil {
		return err
	}

	d.activeContainer = contInfo.ID
	return nil
}
