## coreos run

Starts a new CoreOS instance

### Synopsis


Starts a new CoreOS instance

```
coreos run
```

### Options

```
      --cdrom="": append an CDROM (.iso) to VM
      --channel="alpha": CoreOS channel
      --cloud_config="": cloud-config file location (either a remote URL or a local path)
      --cpus=1: VM's vCPUS
  -d, --detached[=false]: starts the VM in detached (background) mode
  -l, --local[=false]: consumes whatever image is `latest` locally instead of looking online unless there's nothing available.
      --memory=1024: VM's RAM, in MB, per instance (1024 < memory < 3072)
  -n, --name="": names the VM. (if absent defaults to VM's UUID)
      --root="": append a (persistent) root volume to VM
      --sshkey="": VM's default ssh key
      --tap="": append tap interface to VM
      --uuid="random": VM's UUID
      --version="latest": CoreOS version
      --volume=[]: append disk volumes to VM
```

### Options inherited from parent commands

```
      --debug[=false]: adds extra verbosity, and options, for debugging purposes and/or power users
```

### SEE ALSO
* [coreos](coreos.md)	 - CoreOS, on top of OS X and xhyve, made simple.

