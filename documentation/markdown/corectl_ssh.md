## corectl ssh

Attach to or run commands inside a running CoreOS instance

### Synopsis


Attach to or run commands inside a running CoreOS instance

```
corectl ssh
```

### Examples

```
  corectl ssh VMid                 // logins into VMid
  corectl ssh VMid "some commands" // runs 'some commands' inside VMid and exits
```

### Options inherited from parent commands

```
      --debug[=false]: adds extra verbosity, and options, for debugging purposes and/or power users
```

### SEE ALSO
* [corectl](corectl.md)	 - CoreOS over OSX made simple.

