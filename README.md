# CoreOS + xhyve

**WARNING**: xhyve is a very new project, expect bugs! You must be running OS X Yosemite for this to work.

## Step by Step Instructions

## Get a copy of xhyve

```
$ git clone https://github.com/mist64/xhyve
$ make
$ sudo cp build/xhyve /usr/local/bin/
$ xhyve -h
Usage: xhyve [-behuwxACHPWY] [-c vcpus] [-g <gdb port>] [-l <lpc>]
...
```

## Download and run CoreOS 

These two commands will fetch a CoreOS Alpha image, verify it with the build public key, then run it under xhyve.

```
coreos-xhyve-fetch
sudo coreos-xhyve-run
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


