# CoreOS + [xhyve](https://github.com/mist64/xhyve)

**WARNING**
-----------
 - xhyve is a very new project, expect bugs! You must be running OS X 10.10.3 Yosemite or later and 2010 or later Mac for this to work.
 - if you use any version of VirtualBox prior to 5.0 then xhyve will crash your system either if VirtualBox is running or had been run previously after the last reboot (see xhyve's issues [#5](https://github.com/mist64/xhyve/issues/5) and [#9](https://github.com/mist64/xhyve/issues/9) for the full context). So, if you are unable to update VirtualBox to version 5, or later, and were using it in your current session please do restart your Mac before attempting to run xhyve.

## Step by Step Instructions

### Install xhyve
#### from [homebrew](http://brew.sh) (recommended)
```
$ brew install xhyve
```
#### or from [source](https://github.com/mist64/xhyve)
```
$ git clone https://github.com/mist64/xhyve
$ cd xhyve
$ make
$ sudo cp build/xhyve /usr/local/bin/
```
#### check it is working...
```
$ xhyve -h
Usage: xhyve [-behuwxACHPWY] [-c vcpus] [-g <gdb port>] [-l <lpc>] ...
```

### Download and run CoreOS

By default, the following commands will fetch the latest CoreOS Alpha image
available, verify it (if you have gpg installed in your system) with the build
public key, and then run it under xhyve.

```
coreos-xhyve-fetch
sudo coreos-xhyve-run
```

In your terminal you should see something like this:

```
This is localhost (Linux x86_64 4.0.3) 02:59:17
SSH host key: 92:2e:78:25:8e:81:f3:74:61:c7:3b:79:db:3b:0f:c2 (DSA)
SSH host key: 55:19:07:2c:44:9d:0c:f8:61:9e:95:97:61:ab:c5:c5 (ED25519)
SSH host key: ba:69:da:37:7e:c2:b6:26:e4:72:b5:94:d4:b8:97:bb (RSA)
eth0: 192.168.64.1 fe80::24d7:36ff:fe1d:cf32

localhost login: core (automatic login)

CoreOS stable (695.0.0)
Update Strategy: No Reboots
Last login: Thu Jun 11 02:59:17 +0000 2015 on /dev/tty1.
core@localhost ~ $
```

Now you can try to ssh in:

```
$ ssh core@192.168.64.1
```

Or try out docker:

```
$ brew install docker
$ docker -H 192.168.64.1:2375
```

Or try out rkt:

```
$ systemd-run rkt --insecure-skip-verify run coreos.com/etcd,version=v2.0.10 -- --listen-client-urls 'http://0.0.0.0:2379,http://0.0.0.0:4001'
```

And test from your laptop:

```
$ curl 192.168.64.1:2379/version
etcd 2.0.10
```

### Customize

The `coreos-xhyve-fetch` and `coreos-xhyve-run` behavior can be customized
through the following environment variables:
- **XHYVE**  
  defaults to `xhyve`.  
  sets the absolute location (or name, in which case it will search in the $PATH) of the default *xhyve* binary to use.
- **CHANNEL**  
  defaults to `alpha`.  
  available alternatives are `stable` and `beta`
- **VERSION**  
  defaults to `latest`.
- **CPUS**  
  defaults to `1`.
- **MEMORY**  
  defaults to `1024`.  
  value is understood as being in MB.
- **UUID**
  defaults to a random `uuid`.  
  set to a constant value in order to achieve the same IP address across VM reboots.
- **SSHKEY**  
  defaults to `none`
  if set it will add, on startup, the given SSH public key to the *core*
  user's authorized_keys file (it is usually in ~/.ssh/id_rsa.pub).  
  ```
  sudo coreos-xhyve-run SSHKEY="ssh-rsa AAAAB3...== x@y.z" ...
  ```
- **ROOT_HDD**  
  defaults to `none`.
  if set to the absolute path of a pre-formated ext4 disk image, then the
  provided image will be used for a writable root partition, allowing data to
  persist across reboots of the VM.

  **creating a disk image**:
  ```
  dd if=/dev/zero of=./xhyve.img bs=1024k count=5000
  /usr/local/opt/e2fsprogs/sbin/mkfs.ext4 -L ROOT xhyve.img
  ```
  *note: this requires you to install e2fsprogs (`brew install e2fsprogs`)*
- **EXTRA_ARGS**  
  defaults to `none`.  
  used to manually set additional VM parameters that do not fit elsewhere (tap devices, etc).
- **CLOUD_CONFIG**  
  defaults to `https://raw.githubusercontent.com/coreos/coreos-xhyve/master/cloud-init/docker-only.txt`  
  has to be a valid, reachable, URL, pointing to a valid
  [cloud-config](https://coreos.com/docs/cluster-management/setup/cloudinit-cloud-config/)
  file.

  > **tip**:  
  > see [here](https://discussions.apple.com/docs/DOC-3083) for how to
  > host your custom *cloud-config* locally, so that you can run CoreOS locally
  > without any online dependencies, then on
  > `/etc/apache2/users/<YourUsername>.conf` replace `Allow from localhost` by
  > `Allow from localhost, 192.168.0.0/255.255.0.0`.  
  > usage would be something like...  
  > `CLOUD_CONFIG=http://192.168.64.1/~am/coreos-xhyve/xhyve.cloud-init ./coreos-xhyve-run`

For any given VM you can define all your custom settings in a file and then
just consume it like `coreos-xhyve-run -f custom.conf`.
See [here](custom.conf) for an example.
