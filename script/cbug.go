package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

func ifErr(err error, errMsg string, printErr bool) {
	if err != nil /*&& err.Error() != "exit status 1" */ {
		if printErr {
			fmt.Println(errMsg + err.Error())
		} else {
			fmt.Println(errMsg)
		}
		os.Exit(1)
	}
}

func main() {

	type configStruct struct {
		ContainerName    string `json:"containerName"`
		DefaultBehaviour string `json:"exitBehaviourDefault"`
	}
	execLoc, err := os.Executable()
	ifErr(err, "Error getting location of cbug: ", true)
	if _, err = os.Stat(filepath.Dir(execLoc) + "/config.json"); errors.Is(err, os.ErrNotExist) {
		os.WriteFile(filepath.Dir(execLoc)+"/config.json", []byte(`{"containerName": "cbug","exitBehaviourDefault": "shutdown"}`), 0644)
		fmt.Println("test")
	}
	confFile, err := ioutil.ReadFile(filepath.Dir(execLoc) + "/config.json")
	conf := configStruct{}
	_ = json.Unmarshal([]byte(confFile), &conf)

	//arg parsing
	type flagStruct struct {
		keepalive bool
		pause     bool
		stop      bool
		sync      bool
		tty       bool
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
			if !flags.pause && !flags.stop {
				flags.keepalive = true
			} else {
				fmt.Println("Conflicting flags present")
				os.Exit(1)
			}
		case "-p", "--pause":
			if !flags.keepalive && !flags.stop {
				flags.pause = true
			} else {
				fmt.Println("Conflicting flags present")
				os.Exit(1)
			}
		case "-s", "--shutdown":
			if !flags.keepalive && !flags.pause {
				flags.stop = true
			} else {
				fmt.Println("Conflicting flags present")
				os.Exit(1)
			}
		case "-S", "--sync":
			flags.sync = true
		case "-t", "--tty":
			flags.tty = true
		default:
			fmt.Println("Unknown cbug flag \"" + flag + "\"")
			os.Exit(1)
		}
	}
	if !flags.pause && !flags.keepalive && !flags.stop {
		switch conf.DefaultBehaviour {
		case "pause":
			flags.pause = true
		case "keep-alive":
			flags.keepalive = true
		default:
			flags.stop = true
		}
	}

	//should run help command (and maybe others so its in switch) before touching docker
	switch args[0] {
	case "help":
		fmt.Println("USAGE:\n" +
			"\tclean: removes all files from the cbug container.\n" +
			"\tsync: runs clean and then copies the current directory to cbug.\n" +
			"\tconfig: configure the default behaviour of cbug.\n" +
			"\tdefault: if none of these commands are present, the command will be passed\n" +
			"\t         directly to the cbug container.\n" +
			"FLAGS:\n" +
			"\t*flags only work when passing commands to the cbug container, not on " +
			"internal commands*\n" +
			"\t-k, --keep-alive: do not pause or shut down the container when cbug exits\n" +
			"\t-s, --shutdown: shut down the container when cbug exits\n" +
			"\t-p, --pause: pause those container when cbug exits\n" +
			"\t-S, --sync: sync files before running command given\n" +
			"\t-t, --tty: run commands through a tty shell. good for formatting, but will break streaming files into stdin (e.g. using < input.txt)")
		return
	case "config":
		if len(args) > 1 && args[1] == "default" {
			conf.ContainerName = "cbug"
			conf.DefaultBehaviour = "shutdown"
			newConfigJson, err := json.Marshal(conf)
			ifErr(err, "Error sending new config to config file", false)
			os.WriteFile(filepath.Dir(execLoc)+"/config.json", newConfigJson, 0644)
			fmt.Println("reset cbug to its default configuration")
			return
		}
		fmt.Print("New cbug container name (leave empty to remain as \"" + conf.ContainerName + "\"): ")
		var newContainerName string
		fmt.Scanf("%s", &newContainerName)
		if newContainerName != "" {
			conf.ContainerName = newContainerName
		}
		fmt.Print("New container default behaviour (shutdown, pause, or keep-alive. Leave black to remain as \"" + conf.DefaultBehaviour + "\"): ")
		var newBehaviour string
		fmt.Scanf("%s", &newBehaviour)
		switch newBehaviour {
		case "shutdown", "pause", "keep-alive":
			conf.DefaultBehaviour = newBehaviour
		case "":
			break
		default:
			fmt.Println("unrecognized behaviour")
			return
		}
		newConfigJson, err := json.Marshal(conf)
		ifErr(err, "Error sending new config to config file", false)
		os.WriteFile(filepath.Dir(execLoc)+"/config.json", newConfigJson, 0644)

		return
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
			if name == "/"+conf.ContainerName {
				if strings.Contains(dContainer.Image, "eleanormally/cpp-memory-debugger") {
					containerID = dContainer.ID
				} else {
					fmt.Println("Error: found a docker container with the name \"" + conf.ContainerName + "\" in use not by cbug.\nPlease rename/delete the container named \"" + conf.ContainerName + "\", or use \"cbug config\" to change the name of cbug's container")
					return
				}
				break
			}
		}
	}
	if containerID == "" {

		//TODO: add new upgrade command to repull latest docker image, and also pull here if not found any

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
			&specs.Platform{},
			conf.ContainerName,
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
	} else if flags.stop {
		defer func() {
			fmt.Print("Stopping cbug container... ")
			delay := time.Duration(1) * time.Second
			dockerCli.ContainerStop(context.Background(), containerID, &delay)
			fmt.Println("Done")
		}()
	}

	switch args[0] {
	case "clean":
		fmt.Print("Cleaning container... ")
		err := exec.Command("docker", strings.Split("exec "+conf.ContainerName+" bash /custom/removeAll.sh", " ")...).Run()
		ifErr(err, "Error cleaning container: ", true)
		fmt.Println("Done")
	case "sync":
		fmt.Print("Syncing files between current directory and cbug... ")
		err := exec.Command("docker", strings.Split("exec "+conf.ContainerName+" bash /custom/removeAll.sh", " ")...).Run()
		ifErr(err, "Error cleaning container: ", true)
		fmt.Println("Done")
		workdir, err := os.Getwd()
		ifErr(err, "Error accessing current directory", false)
		err = exec.Command("docker", "cp", workdir+"/.", conf.ContainerName+":debugger").Run()
		ifErr(err, "Error copying files to docker container: ", true)
	default:

		if flags.sync {
			fmt.Print("Syncing files between current directory and cbug... ")
			err = exec.Command("docker", strings.Split("exec "+conf.ContainerName+" rm -rf -- *", " ")...).Run()
			ifErr(err, "Error cleaning container: ", true)
			fmt.Println("Done")
			workdir, err := os.Getwd()
			ifErr(err, "Error accessing current directory", false)
			err = exec.Command("docker", "cp", workdir+"/.", conf.ContainerName+":debugger").Run()
			ifErr(err, "Error copying files to docker container: ", true)
		}

		tty := ""
		if flags.tty {
			tty = "-t "
		}
		comArgs := strings.Split("exec -i "+tty+conf.ContainerName, " ")
		if args[0][0] == '/' {
			args = append([]string{"bash"}, args...)
		}
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
		//BUG: inconsistent signal sending with <input.txt type stuff
		for {
			select {
			case sig := <-signalChan:
				if err := command.Process.Signal(sig); err != nil && err.Error() != "os: process already finished" {
					fmt.Println("Error sending signal from cbug to container")
					return
				}
			case err := <-waitChan:
				var waitStatus syscall.WaitStatus
				if exitErr, ok := err.(*exec.ExitError); ok {
					waitStatus = exitErr.Sys().(syscall.WaitStatus)
					os.Exit(waitStatus.ExitStatus())
				}
				ifErr(err, "Error during connection between cbug and container: ", true)
				break Loop
			}
		}
	}

}
