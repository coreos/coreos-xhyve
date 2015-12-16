## corectl load

Loads CoreOS instances defined in an instrumentation file.

### Synopsis


Loads CoreOS instances defined in an instrumentation file (either in TOML, JSON or YAML format).
VMs are always launched by alphabetical order relative to their names.

```
corectl load
```

### Examples

```
  corectl load profiles/demo.toml
```

### Options inherited from parent commands

```
      --debug[=false]: adds extra verbosity, and options, for debugging purposes and/or power users
```

### SEE ALSO
* [corectl](corectl.md)	 - CoreOS over OSX made simple.

