package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

func main() {
	var args struct {
		InputFiles    []string `arg:"positional,required"`
		RemainOpen    bool     `arg:"-k,--keep-alive" help:"Prevents cppmem from closing the container on completion"`
		DrMemory      bool     `arg:"-m,--drmemory"`
		ContainerName string   `arg:"-n,--name" default:"cppmem"`
		Compiler      string   `arg:"-c,--compiler" default:"g++"`
		CompilerFlags []string `arg:"-f,--compiler-flags" help:"flags to pass to the compiler"`
		DebuggerFlags []string `arg:"-d,--debugger-flags" help:"flags to pass to the memory debugger"`
		DebuggerArgs  []string `arg:"-a,--args" help:"arguments to pass to the program"`
	}
	arg.MustParse(&args)

	if args.DrMemory {
		fmt.Println("DrMemory is not currently supported in cppmem.")
		return
	}

	if len(args.InputFiles) == 0 {
		fmt.Println("No source files listed to compile.")
		return
	}

	//initializing docker container
	dockerCli, err := client.NewEnvClient()
	if err != nil {
		fmt.Println("Unable to connect to docker. Have you installed docker?")
		return
	}

	containers, err := dockerCli.ContainerList(context.Background(), types.ContainerListOptions{
		All: true,
	})
	if err != nil {
		panic(err)
	}

	containerID := ""
	for _, dContainer := range containers {
		for _, name := range dContainer.Names {
			if name == "/"+args.ContainerName {
				if strings.Contains(dContainer.Image, "eleanormally/cpp-memory-debugger") {
					containerID = dContainer.ID
				} else {
					fmt.Println("ERROR: found a docker container with this name in use not by cppdebug.\nPlease specify a name with the -n flag, or delete the container named \"" + args.ContainerName + "\"")
					return
				}
				break
			}
		}
	}
	if containerID == "" {
		fmt.Print("Creating New Docker Container... ")
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
			args.ContainerName,
		)
		if err != nil {
			fmt.Println("\n\nError creating Docker container: " + err.Error())
			return
		}
		fmt.Print("Done\n\n")

		containerID = cont.ID
	}

	// forcing container on
	containerInfo, err := dockerCli.ContainerInspect(context.Background(), containerID)
	if err != nil {
		fmt.Println("Error inspecting Docker container: " + err.Error())
		return
	}
	if containerInfo.State.Paused {
		err = dockerCli.ContainerUnpause(context.Background(), containerID)
		if err != nil {
			fmt.Println("Error unpausing Docker container: " + err.Error())
			return
		}
	} else if !containerInfo.State.Running {
		dockerCli.ContainerStart(context.Background(), containerID, types.ContainerStartOptions{})
	}

	//allows for close on cancel
	if !args.RemainOpen {
		delay := time.Duration(1) * time.Second
		defer dockerCli.ContainerStop(context.Background(), containerID, &delay)
	} else {
		defer dockerCli.ContainerPause(context.Background(), containerID)
	}

	workdir, err := os.Getwd()
	if err != nil {
		fmt.Println("Error accessing current directory.")
		panic(err)
	}

	err = exec.Command("docker", "exec", args.ContainerName, "rm", "-rf", "--", "*").Run()
	if err != nil {
		fmt.Println("Error cleaning container directory")
		return
	}
	err = exec.Command("docker", "cp", workdir+"/.", args.ContainerName+":debugger").Run()
	if err != nil {
		fmt.Println("Error copying directory to container: " + err.Error())
		return
	}

	var compileCommand = make([]string, 5+len(args.InputFiles)+len(args.CompilerFlags))
	compileCommand[0] = "exec"
	compileCommand[1] = args.ContainerName
	compileCommand[2] = args.Compiler
	for index, file := range args.InputFiles {
		compileCommand[3+index] = file
	}
	for index, flag := range args.CompilerFlags {
		if len(flag) == 1 {
			compileCommand[3+len(args.InputFiles)+index] = "-" + flag
		} else {
			compileCommand[3+len(args.InputFiles)+index] = "--" + flag
		}
	}
	//TODO: figure out how to parse flags in a better way
	compileCommand[len(compileCommand)-2] = "-o"
	compileCommand[len(compileCommand)-1] = "cppmem.out"

	fmt.Print("Compiling... ")
	out, err := exec.Command("docker", compileCommand...).Output()
	if err != nil {
		fmt.Println("\nCompiler Error:")
		fmt.Println(string(out))
		return
	}
	fmt.Println("Done")

	if args.DrMemory {
		//TODO: drmemory support
	} else {
		debugCommand := []string{"valgrind"}
		debugCommand = append(debugCommand, args.DebuggerFlags...)
		debugCommand = append(debugCommand, "./cppmem.out")
		debugCommand = append(debugCommand, args.DebuggerArgs...)
		execId, err := dockerCli.ContainerExecCreate(context.Background(), args.ContainerName, types.ExecConfig{
			User:         "",
			Privileged:   false,
			Tty:          true,
			AttachStdin:  true,
			AttachStderr: true,
			AttachStdout: true,
			Detach:       false,
			DetachKeys:   "",
			Env:          []string{},
			WorkingDir:   "/debugger",
			Cmd:          debugCommand,
		})
		if err != nil {
			fmt.Println("Error creating valgrind command: " + err.Error())
			return
		}
		execResponse, err := dockerCli.ContainerExecAttach(context.Background(), execId.ID, types.ExecStartCheck{Detach: false, Tty: true})
		if err != nil {
			fmt.Println("Error attaching valgrind to container" + err.Error())
		}
		defer execResponse.Close()

	}

}
