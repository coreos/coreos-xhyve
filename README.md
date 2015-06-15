# CoreOS + [xhyve](mist64/xhyve)

**WARNING**
-----------
 - xhyve is a very new project, expect bugs! You must be running OS X 10.10.3 Yosemite or later and 2010 or later Mac for this to work.
 - xhyve will crash your system if VirtualBox had been run previously (see xhyve's issues [#5](mist64/xhyve#5) and [#9](mist64/xhyve#9) for the full context). So if you were using it in your current session please do restart your Mac before attempting to run xhyve.

## Step by Step Instructions

### Get a copy of xhyve

```
$ git clone https://github.com/mist64/xhyve
$ cd xhyve
$ make
$ sudo cp build/xhyve /usr/local/bin/
$ xhyve -h
Usage: xhyve [-behuwxACHPWY] [-c vcpus] [-g <gdb port>] [-l <lpc>]
...
```

### Download and run CoreOS

By default, the following commands will fetch the latest CoreOS Alpha image
available, verify it (if you have gpg installed in your system) with the build
public key, and then run it under xhyve.

```
coreos-xhyve-fetch
coreos-xhyve-run
```

In your teminal you should see something like this:

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
- **CHANNEL**  
  defaults to `alpha`.  
  available alternatives are `stable` and `beta`
- **VERSION**  
  defaults to `latest`.
- **MEMORY**  
  defaults to `1024`.  
  value is understood as being in MB.
- **CLOUD_CONFIG**  
  defaults to `https://raw.githubusercontent.com/coreos/coreos-xhyve/master/cloud-init/docker-only.txt`.  
  has to be a valid, reachable, URL, pointing to a valid *cloud-config* file.

  > **tip**:  
  > see [here](https://discussions.apple.com/docs/DOC-3083) for how to
  > host your custom *cloud-config* locally, so that you can run CoreOS locally
  > without any online dependencies, then on
  > `/etc/apache2/users/<YourUsername>.conf` replace `Allow from localhost` by
  > `Allow from localhost, 192.168.0.0/255.255.0.0`.  
  > usage would be something like...  
  > `CLOUD_CONFIG=http://192.168.64.1/~am/coreos-xhyve/xhyve.cloud-init ./coreos-xhyve-run`
  >

By default `/Users` is mounted inside your CoreOS VM, as `/Users`, so docker
volumes will work as expected.
