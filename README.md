# CBUG
A shell tool to wrap around a docker container and make compiling and debugging memory easy in a closed environment.
This tool is especially designed for those developing on arm systems (e.g. apple silicon) as most memory debugging tools are not built for it. 

## Installing CBUG
Before installing cbug, make sure that you have installed [docker](https://docker.com). Cbug is built on docker, and will not run without it.

To install cbug, download the latest [release](https://github.com/eleanormally/cbug/releases), and unzip the file. To use cbug, either invoke its path directly (i.e. `~/user/cbug-verion/bin/cbug`), or add the bin directory to your path.

## Using cbug
Using cbug is very simple. Simply add the `cbug` keyword before any command you want to use in the container. Before compiling or running code, however, make sure to run `cbug sync` to sync your current directory's files with cbug.
> if a container does not currently exist, cbug will automatically spin one up for you

### All cbug Commands

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


