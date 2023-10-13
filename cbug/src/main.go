package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/akamensky/argparse"
)

func readJson(filename string, format any, defaultFileValue string) error {
	execLoc, err := os.Executable()
	if err != nil {
		return err
	}

	filePath := filepath.Join(filepath.Dir(execLoc), filename)

	_, err = os.Stat(filePath)
	if os.IsNotExist(err) {
		if defaultFileValue != "" {
			// file does not exist
			err = os.WriteFile(filePath, []byte(defaultFileValue), 0644)
			if err != nil {
				return err
			}
		} else {
			return errors.New("file does not exist")
		}
	} else if err != nil {
		// other error
		return err
	}

	file, err := os.ReadFile(filePath)

	_ = json.Unmarshal([]byte(file), format)

	return nil
}

type Config struct {
	ContainerName string `json:"containerName"`
	ExitBehaviour string `json:"exitBehaviourDefault"`
	ImageName     string `json:"image"`
}

func getConfig() (*Config, error) {
	conf := Config{}

	err := readJson("config.json", &conf, `{"containerName": "cbug","exitBehaviourDefault": "shutdown","image":"eleanormally/cpp-memory-debugger"}`)

	if err != nil {
		return nil, err
	}

	return &conf, nil
}

type Info struct {
	Version      string `json:"version"`
	Platform     string `json:"target"`
	Architecture string `json:"architecture"`
}

func addBasicDockerArgs(command *argparse.Command) {
	command.Flag("n", "name", &argparse.Options{Required: false, Help: "name of container"})
	command.Flag("x", "x86", &argparse.Options{Required: false, Help: "force cbug to use x86 (emulated if necessary)"})
	command.Flag("a", "arm", &argparse.Options{Required: false, Help: "force cbug to use arm (emulated if necessary)"})
}

func getInfo() (*Info, error) {
	info := Info{}

	err := readJson("release-info.json", &info, "")
	if err != nil {
		return nil, err
	}

	return &info, nil
}

func main() {
	conf, err := getConfig()
	if err != nil {
		panic(err)
	}

	info, err := getInfo()
	if err != nil {
		panic(err)
	}

	_ = conf
	_ = info

	parser := argparse.NewParser("cbug", "A standardised environment for debugging")

	helpCmd := parser.NewCommand("help", "show help")

	infoCmd := parser.NewCommand("info", "show cbug information")

	syncCmd := parser.NewCommand("sync", "sync current directory to cbug")
	addBasicDockerArgs(syncCmd)

	execCmd := parser.NewCommand("shell", "execute code in cbug")
	addBasicDockerArgs(execCmd)

	attachCmd := parser.NewCommand("attach", "attach current terminal to cbug")
	addBasicDockerArgs(attachCmd)

	err = parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
		return
	}

	if helpCmd.Happened() {
		fmt.Print(parser.Usage(err))
		return
	}

	if infoCmd.Happened() {
		fmt.Println("               version:", info.Version)
		fmt.Println("              platform:", info.Platform)
		fmt.Println("          architecture:", info.Architecture)
		fmt.Println("        container name:", conf.ContainerName)
		fmt.Println("default exit behaviour:", conf.ExitBehaviour)
		return
	}

	if attachCmd.Happened() {
		d, err := NewDocker()
		if err != nil {
			panic(err)
		}
		err = d.setActiveContainer(conf.ContainerName, &GenerateContainerOpts{
			image:    conf.ImageName,
			name:     conf.ContainerName,
			platform: info.Platform,
		})
		if err != nil {
			panic(err)
		}
		info, err := d.client.ContainerInspect(context.Background(), d.activeContainer)
		if err != nil {
			panic(err)
		}
		fmt.Println(info)
	}

}
