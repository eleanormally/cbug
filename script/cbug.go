package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
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

	"github.com/google/go-github/v50/github"
)

type flagStruct struct {
	keepalive bool
	pause     bool
	stop      bool
	sync      bool
	tty       bool
	forceX86  bool
	forceArm  bool
}

type configStruct struct {
	ContainerName    string `json:"containerName"`
	DefaultBehaviour string `json:"exitBehaviourDefault"`
	DockerContainer  string `json:"imageName"`
}

type infoStruct struct {
	Version            string `json:"version"`
	Platform           string `json:"target"`
	ArchitectureString string `json:"architecture"`
	Tag                DTag
}

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

func selectedTag(info infoStruct, flags flagStruct) string {
	if flags.forceArm {
		return string(DTagArm)
	}
	if flags.forceX86 {
		return string(DTagX86)
	}
	return string(info.Tag)
}

func doImagePull(dockerCli *client.Client, image string) bool {
	closer, err := dockerCli.ImagePull(context.Background(), image, types.ImagePullOptions{
		All:          false,
		RegistryAuth: "",
		Platform:     "",
	})
	var in = make([]byte, 8)
	fullStatus := ""
	for {
		_, err := closer.Read(in)
		if err == io.EOF {
			break
		}
		fullStatus = fullStatus + string(in)
	}
	if strings.Contains(fullStatus, "up to date") {
		return false
	}

	ifErr(err, "Error pulling docker image: ", true)
	closer.Close()
	return true
}

func unzip(source string, destination string) error {
	reader, err := zip.OpenReader(source)
	ifErr(err, "Error unzipping new version", false)
	defer reader.Close()
	ifErr(err, "Error unzipping new version", false)
	for _, f := range reader.File {
		err := unzipFile(f, destination)
		if err != nil {
			return err
		}
	}
	return nil
}

func unzipFile(f *zip.File, destination string) error {
	//this error checking is for a weird file that macos likes to add to its zips
	//given that I use a mac, releases are going to end up having this, and it
	//doesnt break anything else
	fmt.Println(f.Name)
	if strings.Contains(f.Name, "__MACOS") || strings.Contains(f.Name, ".DS_Store") {
		return nil
	}
	filePath := filepath.Join(destination, f.Name)
	if !strings.HasPrefix(filePath, filepath.Clean(destination)+string(os.PathSeparator)) {
		return fmt.Errorf("invalid file path %s", filePath)
	}
	if f.FileInfo().IsDir() {
		if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
			return err
		}
		return nil
	}

	destinationFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	zippedFile, err := f.Open()
	if err != nil {
		return err
	}
	defer zippedFile.Close()

	if _, err := io.Copy(destinationFile, zippedFile); err != nil {
		return err
	}
	return nil
}

func getTag(instr string) (DTag, error) {
	switch instr {
	case "arm64":
		return DTagArm, nil
	case "x86":
		return DTagX86, nil
	}
	return "", errors.New("unrecognized architecture")
}

type DTag string

const (
	DTagX86 DTag = "x86"
	DTagArm DTag = "latest"
)

func (t DTag) readable() string {
	switch t {
	case DTagX86:
		return "x86"
	case DTagArm:
		return "arm"
	default:
		return ""
	}
}

func getNewRelease(releaseInfo infoStruct) bool {
	fmt.Println("Checking for new cbug version...")
	gitCli := github.NewClient(nil)
	releases, _, err := gitCli.Repositories.ListReleases(context.Background(), "eleanormally", "cbug", &github.ListOptions{
		Page:    0,
		PerPage: 1,
	})
	ifErr(err, "Error finding cbug repository. Are you connected to the internet?", false)
	if releases[0].GetTagName() != releaseInfo.Version {
		fmt.Print("Found new version of cbug, would you like to install it? [Y/n]: ")
		var res string
		fmt.Scan(&res)
		if strings.ToLower(res)[0] != 'y' {
			return true
		}
		assets, _, err := gitCli.Repositories.ListReleaseAssets(context.Background(), "eleanormally", "cbug", releases[0].GetID(), &github.ListOptions{
			Page:    0,
			PerPage: 5,
		})
		ifErr(err, "Error getting assets from latest version of cbug", false)
		for _, asset := range assets {
			if len(asset.GetName()) > 4 && asset.GetName()[len(asset.GetName())-4:] == ".zip" && strings.Contains(asset.GetName(), releaseInfo.Platform) {
				fmt.Print("Downloading...")
				file, err := os.Create(os.TempDir() + "/" + asset.GetName())
				resp, err := http.Get(asset.GetBrowserDownloadURL())
				ifErr(err, "Error downloading latest release", false)
				defer resp.Body.Close()
				_, err = io.Copy(file, resp.Body)
				ifErr(err, "Error copying file: ", true)
				defer file.Close()
				execLoc, err := os.Executable()
				ifErr(err, "Error getting current executable path: ", true)
				parentfolder := filepath.Dir(filepath.Dir(execLoc))
				if !strings.Contains(filepath.Base(parentfolder), "cbug") {
					fmt.Println("Error, please make sure the cbug folder is named \"cbug\". This is for file security purposes.")
					return false
				}
				err = os.Rename(parentfolder, parentfolder+"old")
				ifErr(err, "Error renaming old version of cbug", false)
				if err = unzip(os.TempDir()+"/"+asset.GetName(), filepath.Dir(parentfolder)); err != nil {
					fmt.Println(err)
					fmt.Println("Error installing new version of cbug, reverting to old version...")
					os.RemoveAll(parentfolder)
					os.Remove(parentfolder)
					os.Rename(parentfolder+"old", parentfolder)
					return false
				}
				os.RemoveAll(parentfolder + "old/")
				os.Remove(parentfolder + "old")
				fmt.Printf("Done")
				return true
			}
		}
		fmt.Println("Did not find a valid download file in latest release for this computer. No update to the cbug script was performed.")
	}
	return true
}

func main() {

	execLoc, err := os.Executable()
	ifErr(err, "Error getting location of cbug: ", true)
	if _, err = os.Stat(filepath.Dir(execLoc) + "/../config.json"); errors.Is(err, os.ErrNotExist) {
		os.WriteFile(filepath.Dir(execLoc)+"/../config.json", []byte(`{"containerName": "cbug","exitBehaviourDefault": "shutdown"}`), 0644)
	}
	confFile, err := ioutil.ReadFile(filepath.Dir(execLoc) + "/../config.json")
	conf := configStruct{}
	_ = json.Unmarshal([]byte(confFile), &conf)

	//arg parsing

	flags := flagStruct{}

	args := []string{}

	osArgs := []string{}
	if len(os.Args) == 1 {
		osArgs = []string{"help"}
	} else {
		osArgs = os.Args[1:]
	}

	//flag handling
	oneTimeContainerName := false
	var flagSlice = []string{}
	for index, arg := range osArgs {
		if oneTimeContainerName {
			oneTimeContainerName = false
			conf.ContainerName = arg
			continue
		}
		if arg == "-n" || arg == "--name" {
			oneTimeContainerName = true
		} else if arg[0] != '-' {
			args = osArgs[index:]
			break
		} else {
			flagSlice = append(flagSlice, arg)
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
		case "-a", "--arm":
			if flags.forceX86 {
				fmt.Println("cannot force x86 and force arm")
				os.Exit(1)
			}
			flags.forceArm = true
		case "-x", "--x86":
			if flags.forceArm {
				fmt.Println("cannot force x86 and force arm")
				os.Exit(1)
			}
			flags.forceX86 = true
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

	if _, err = os.Stat(filepath.Dir(execLoc) + "/../release-info.json"); errors.Is(err, os.ErrNotExist) {
		fmt.Println("CRITICAL ERROR: no release-info file. Please re-add this file or redownload cbug.")
		os.Exit(1)
	}
	infoFile, err := ioutil.ReadFile(filepath.Dir(execLoc) + "/../release-info.json")
	releaseInfo := infoStruct{}
	_ = json.Unmarshal([]byte(infoFile), &releaseInfo)
	releaseInfo.Tag, err = getTag(releaseInfo.ArchitectureString)
	ifErr(err, "Error: ", true)

	if len(args) == 0 {
		fmt.Println("Error, no argument given to cbug")
		os.Exit(1)
	}

	//should run help command (and maybe others so its in switch) before touching docker
	switch args[0] {
	case "help":
		fmt.Println("USAGE:\n" +
			"\tclean: removes all files from the cbug container.\n" +
			"\tsync: runs clean and then copies the current directory to cbug.\n" +
			"\tconfig: configure the default behaviour of cbug.\n" +
			"\tremove [name]: remove container with [name]. If no name is given, it will remove the default container. Will not remove non cbug containers.\n" +
			"\tattach: attach current terminal to the cbug container. Useful for executing many commands back to back\n" +
			"\tdefault: if none of these commands are present, the command will be passed\n" +
			"\tupgrade: check for updates to cbug\n" +
			"\tinfo: view information on cbug\n" +
			"\t         directly to the cbug container.\n" +
			"FLAGS:\n" +
			"\t*flags only work when passing commands to the cbug container, not on " +
			"internal commands*\n" +
			"\t-k, --keep-alive: do not pause or shut down the container when cbug exits\n" +
			"\t-s, --shutdown: shut down the container when cbug exits\n" +
			"\t-p, --pause: pause those container when cbug exits\n" +
			"\t-S, --sync: sync files before running command given\n" +
			"\t-t, --tty: run commands through a tty shell. good for formatting, but will break streaming files into stdin (e.g. using < input.txt)\n" +
			"\t-n, --name: change the name of the container for this command. Does not effect the default config" +
			"\t-x, --x86: force cbug to use an x86 container (works on all machines). If used on an existing arm container, it will not work.\n" +
			"\t-a, --arm: force cbug to use an arm container (works on all machines). If used on an existing x86 container, it will not work.")
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
		os.WriteFile(filepath.Dir(execLoc)+"/../config.json", newConfigJson, 0644)

		return
	case "remove":
		//NOTE: this cannot implement the whole command because needs docker
		if len(args) > 1 {
			conf.ContainerName = args[1]
		}
	case "info":
		fmt.Println("cbug version: " + releaseInfo.Version)
		return
	}

	//starting handling docker

	dockerCli, err := client.NewEnvClient()
	ifErr(err, "Unable to connect to docker. Have you installed docker on your machine and is it running?", false)

	if args[0] == "upgrade" {
		if releaseInfo.Version == "dev" {
			fmt.Println("You are currently on a development version of cbug, so no updates are allowed.")
			return
		}
		fmt.Println("checking for and downloading new docker containers...")
		images, err := dockerCli.ImageList(context.Background(), types.ImageListOptions{All: true})
		ifErr(err, "Error listing docker images", false)
		anyNew := false
		for _, image := range images {
			for _, label := range image.RepoTags {
				if strings.Contains(label, "eleanormally/cpp-memory-debugger") {
					if doImagePull(dockerCli, label) {
						fmt.Println("Upgraded docker image " + label + ". THIS HAS NOT UPGRADED ANY CBUG CONTAINERS. Please remove all existing cbug containers and recreate them to use the new version.")
						anyNew = true
					}
					break
				}
			}
		}
		if !anyNew {
			fmt.Println("Already on latest container")
		}
		if !getNewRelease(releaseInfo) {
			os.Exit(1)
		}
		return
	}

	containers, err := dockerCli.ContainerList(context.Background(), types.ContainerListOptions{
		All: true,
	})
	ifErr(err, "Docker Error: ", true)

	containerID := ""
	for _, dContainer := range containers {
		for _, name := range dContainer.Names {
			if name == "/"+conf.ContainerName {
				fmt.Println(dContainer.Image)
				fmt.Println(dContainer)
				//NOTE: the sha256 case is for windows, which (as of feb 2023) does not properly return the image name on this
				// command. currently it returns the ID, along with dContainer.ID. If this changes, the or can be removed
				if strings.Contains(dContainer.Image, "eleanormally/cpp-memory-debugger") || strings.Contains(dContainer.Image, "sha256") {
					//remove needs to be up here so that don't accidentally create new container if name not found
					if args[0] == "remove" {
						delay := time.Duration(1) * time.Millisecond
						dockerCli.ContainerStop(context.Background(), dContainer.ID, &delay)
						fmt.Print("Removing container... ")
						err := dockerCli.ContainerRemove(context.Background(), dContainer.ID, types.ContainerRemoveOptions{
							RemoveVolumes: true,
							RemoveLinks:   false,
							Force:         false,
						})
						ifErr(err, "Error removing container: ", true)
						fmt.Println("Done")
						return
					}
					if (!flags.forceArm && !flags.forceX86) || strings.Contains(dContainer.Image, selectedTag(releaseInfo, flags)) {
						containerID = dContainer.ID
					} else {
						if flags.forceArm {
							fmt.Println("Error: This container is for x86, please remove this container or specify a name for a new container.")
							os.Exit(1)
						}
						if flags.forceX86 {
							fmt.Println("Error: This container is for arm, please remove this container or specify a name for a new container.")
							os.Exit(1)
						}
					}
				} else {
					fmt.Println("Error: found a docker container with the name \"" + conf.ContainerName + "\" in use not by cbug.\nPlease rename/delete the container named \"" + conf.ContainerName + "\", or use \"cbug config\" to change the name of cbug's container")
					return
				}
				break
			}
		}
	}
	if args[0] == "remove" {
		fmt.Println("Could not find container \"" + conf.ContainerName + "\"")
		return
	}
	if containerID == "" {

		images, err := dockerCli.ImageList(context.Background(), types.ImageListOptions{
			All: true,
		})
		ifErr(err, "Error listing docker images", false)
		foundImage := false
	mainLoop:
		for _, image := range images {
			for _, label := range image.RepoDigests {
				if strings.Contains(label, "eleanormally/cpp-memory-debugger:"+selectedTag(releaseInfo, flags)) {
					foundImage = true
					break mainLoop
				}
			}
		}
		if !foundImage {
			fmt.Print("Pulling cbug docker image...")
			doImagePull(dockerCli, "eleanormally/cpp-memory-debugger:"+selectedTag(releaseInfo, flags))
			fmt.Println("Done")
		}

		fmt.Print("Creating New Docker Container...")
		platform := specs.Platform{}
		if selectedTag(releaseInfo, flags) != string(releaseInfo.Tag) {
			switch selectedTag(releaseInfo, flags) {
			case string(DTagArm):
				platform.Architecture = "arm64"
			case string(DTagX86):
				platform.Architecture = "amd64"
			}
			platform.OS = "linux"
		}

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
				Healthcheck:     &container.HealthConfig{},
				ArgsEscaped:     false,
				Image:           "eleanormally/cpp-memory-debugger:" + selectedTag(releaseInfo, flags),
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
			&platform,
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

		var comArgs = []string{}
		if args[0] == "attach" {
			if tty != "" {
				fmt.Println("tty is not possible when attaching a container. ignoring...")
			}
			comArgs = []string{"attach", conf.ContainerName}
			args = []string{}
		} else {
			comArgs = strings.Split("exec -i "+tty+conf.ContainerName, " ")
			if args[0][0] == '/' {
				args = append([]string{"bash"}, args...)
			}
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
