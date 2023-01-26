package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

func ifErr(err error, errMsg string, printErr bool) {
	if err != nil {
		if printErr {
			fmt.Println(errMsg + err.Error())
		} else {
			fmt.Println(errMsg)
		}
		os.Exit(1)
	}
}

func main() {
	//arg parsing

	type flagStruct struct {
		keepalive bool
		pause     bool
		sync      bool
	}

	flags := flagStruct{}

	args := os.Args[1:]

	//flag handling
	var flagSlice = make([]string, 0, 5)
	for index, arg := range args {
		if arg[0] != '-' {
			flagSlice = args[:index]
			args = args[index:]
			break
		}
	}

	for _, flag := range flagSlice {
		switch flag {
		case "-k", "--keep-alive":
			if !flags.pause {
				flags.keepalive = true
			} else {
				fmt.Println("Conflicting flags present: Cannot both pause and stop container")
				os.Exit(1)
			}
		case "-p", "--pause":
			if !flags.keepalive {
				flags.pause = true
			} else {
				fmt.Println("Conflicting flags present: Cannot both pause and stop container")
				os.Exit(1)
			}
		case "-s", "--sync":
			flags.sync = true
		default:
			fmt.Println("Unknown cppmem flag \"" + flag + "\"")
			os.Exit(1)
		}
	}

	//starting handling docker

	dockerCli, err := client.NewEnvClient()
	ifErr(err, "Unable to connect to docker. Have you installed docker on your machine?", false)

	containers, err := dockerCli.ContainerList(context.Background(), types.ContainerListOptions{
		All: true,
	})
	ifErr(err, "Docker Error: ", true)

	containerID := ""
	for _, dContainer := range containers {
		for _, name := range dContainer.Names {
			if name == "/cppmem" {
				if strings.Contains(dContainer.Image, "eleanormally/cpp-memory-debugger") {
					containerID = dContainer.ID
				} else {
					fmt.Println("Error: found a docker container with the name \"cppmem\" in use not by cppmem.\nPlease rename or delete the container named \"cppmem\"")
					return
				}
				break
			}
		}
	}
	if containerID == "" {
		fmt.Print("Creating New Docker Container...")
		cont, err := dockerCli.ContainerCreate(
			context.Background(),
			&container.Config{
				Hostname:        "",
				Domainname:      "",
				User:            "",
				AttachStdin:     true,
				AttachStdout:    true,
				AttachStderr:    true,
				ExposedPorts:    nil,
				Tty:             true,
				OpenStdin:       true,
				StdinOnce:       false,
				Env:             []string{},
				Cmd:             []string{},
				Healthcheck:     &container.HealthConfig{},
				ArgsEscaped:     false,
				Image:           "eleanormally/cpp-memory-debugger:latest",
				Volumes:         map[string]struct{}{},
				WorkingDir:      "/debugger",
				Entrypoint:      []string{},
				NetworkDisabled: false,
				MacAddress:      "",
				OnBuild:         []string{},
				Labels:          map[string]string{},
				StopSignal:      "",
				StopTimeout:     new(int),
				Shell:           []string{},
			},
			&container.HostConfig{},
			nil,
			&specs.Platform{
				Architecture: "amd64",
				OS:           "linux",
			},
			"cppmem",
		)
		ifErr(err, "\n\nError creating Docker container: ", true)
		fmt.Print("Done\n\n")
		containerID = cont.ID
	}
	containerInfo, err := dockerCli.ContainerInspect(context.Background(), containerID)
	if err != nil {
		fmt.Println("Error inspecting Docker container: ", true)
		return
	}
	if containerInfo.State.Paused {
		err = dockerCli.ContainerUnpause(context.Background(), containerID)
		if err != nil {
			fmt.Println("Error unpausing Docker container: ", true)
			return
		}
	} else if !containerInfo.State.Running {
		dockerCli.ContainerStart(context.Background(), containerID, types.ContainerStartOptions{})
	}

	if flags.pause {
		defer dockerCli.ContainerPause(context.Background(), containerID)
	} else if !flags.keepalive {
		defer func() {
			fmt.Print("Stopping cppmem container... ")
			delay := time.Duration(1) * time.Second
			dockerCli.ContainerStop(context.Background(), containerID, &delay)
			fmt.Println("Done")
		}()
	}

	switch args[0] {
	case "clean":
		fmt.Print("Cleaning container... ")
		err = exec.Command("docker", strings.Split("exec cppmem rm -rf -- *", " ")...).Run()
		ifErr(err, "Error cleaning container: ", true)
		fmt.Println("Done")
	case "sync":
		fmt.Print("Syncing files between current directory and cppmem... ")
		err = exec.Command("docker", strings.Split("exec cppmem rm -rf -- *", " ")...).Run()
		ifErr(err, "Error cleaning container: ", true)
		fmt.Println("Done")
		workdir, err := os.Getwd()
		ifErr(err, "Error accessing current directory", false)
		err = exec.Command("docker", "cp", workdir+"/.", "cppmem:debugger").Run()
		ifErr(err, "Error copying files to docker container: ", true)
	default:

		if flags.sync {
			fmt.Print("Syncing files between current directory and cppmem... ")
			err = exec.Command("docker", strings.Split("exec cppmem rm -rf -- *", " ")...).Run()
			ifErr(err, "Error cleaning container: ", true)
			fmt.Println("Done")
			workdir, err := os.Getwd()
			ifErr(err, "Error accessing current directory", false)
			err = exec.Command("docker", "cp", workdir+"/.", "cppmem:debugger").Run()
			ifErr(err, "Error copying files to docker container: ", true)
		}

		comArgs := strings.Split("exec -i cppmem", " ")
		command := exec.Command("docker", append(comArgs, args...)...)
		command.Stdin = os.Stdin
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		err = command.Start()
		ifErr(err, "Error creating command in container: ", true)
		waitChan := make(chan error, 1)
		go func() {
			waitChan <- command.Wait()
			close(waitChan)
		}()
		signalChan := make(chan os.Signal, 1)
		signal.Notify(signalChan)
	Loop:
		for {
			select {
			case sig := <-signalChan:
				if err := command.Process.Signal(sig); err != nil {
					fmt.Println("Error sending signal from cppmem to container")
					os.Exit(1)
				}
			case err := <-waitChan:
				var waitStatus syscall.WaitStatus
				if exitErr, ok := err.(*exec.ExitError); ok {
					waitStatus = exitErr.Sys().(syscall.WaitStatus)
					os.Exit(waitStatus.ExitStatus())
				}
				ifErr(err, "Error during connection between cppmem and container: ", true)
				break Loop
			}
		}
	}

}
