# CBUG
A shell tool to wrap around a docker container and make compiling and debugging memory easy in a closed environment.
This tool is especially designed for those developing on arm systems (e.g. apple silicon) as most memory debugging tools are not built for it. 

### `NOTE: Cbug is currently only tested and functional for Apple Silicon Macs. This will change in the future, but the program will crash on other machines, even when built from source.`

## Installing CBUG
Before installing cbug, make sure that you have installed [docker](https://docker.com). Cbug is built on docker, and will not run without it.

To install cbug, download the latest [release](https://github.com/eleanormally/cbug/releases), and unzip the file. To use cbug, either invoke its path directly (i.e. `~/user/cbug-verion/bin/cbug`), or add the bin directory to your path.

> See the [wiki](https://github.com/eleanormally/cbug/wiki#cbug-an-easy-to-use-shell-for-debugging) for a more detailed setup guide.

## Using cbug
Using cbug is very simple. Simply add the `cbug` keyword before any command you want to use in the container. Before compiling or running code, however,A make sure to run `cbug sync` to sync your current directory's files with cbug.
> if a container does not currently exist, cbug will automatically spin one up for you 
#### Example usage
```
cbug sync
cbug g++ *.cpp
cbug valgrind ./a.out
```

## FAQ
#### cbug gives me an `exec format error: unknown`
This happens when code is compiled on your computer, but run in cbug. You must remember to compile the program in cbug

#### running cbug for the first time gives me a "cannot check for malicious software error"
Open the executable (the file in the bin folder of cbug) by right clicking, and clicking open. Once you have opened it for the first time like this, you will be able to run it from the command line.

## All cbug Commands

#### `cbug help`
Displays a list of all commands and flags available to use

#### `cbug clean`
Remove all files from current cbug container

#### `cbug sync`
Syncs the current working directory with cbug so that all files between the two are identical.

#### `cbug config`
Allows you to configure the default behaviour of cbug. 
> You can also run `cbug config default` to restore the default configuration

#### `cbug remove`
Removes a cbug container from docker

#### `cbug upgrade`
Cbug will look for updates to itself and the docker container it uses, and download any if available.

#### `cbug info`
Display cbug's version and any other important information


### command flags

These flags are generally only for passing commands into cbug containers and `cbug attach`.

#### `-k`, `--keep-alive`
Force the container to remain running after executing the command.

#### `-s`, `--shutdown`
Force the container to shutdown after executing the command.

#### `-p`, `--pause`
Pause the container after executing the command.

#### `-S`, `--sync`
Sync the files between the current working directory and cbug before running the command.

#### `-n`, `--name`
Run this command on a docker container with a different name than default (this default can also be changed with `cbug config`). This command also works with `cbug sync`

#### `-t`, `--tty`
Force docker to emulate/pass through as a tty shell. This can cause some problems, but if cbug is not displaying the full output of the command or input streaming is not working, this may help. This flag is not available for `cbug attach`.


