## coreos ssh

Attach to or run commands inside a running CoreOS instance

### Synopsis


Attach to or run commands inside a running CoreOS instance

```
coreos ssh
```

### Examples

```
  coreos ssh VMid                 // logins into VMid
  coreos ssh VMid "some commands" // runs 'some commands' inside VMid and exits
```

### Options inherited from parent commands

```
      --debug[=false]: adds extra verbosity, and options, for debugging purposes and/or power users
```

### SEE ALSO
* [coreos](coreos.md)	 - CoreOS, on top of OS X and xhyve, made simple.

