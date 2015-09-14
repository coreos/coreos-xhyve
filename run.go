// Copyright 2015 - António Meireles  <antonio.meireles@reformi.st>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/satori/go.uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	runCmd = &cobra.Command{
		Use:     "run",
		Aliases: []string{"start"},
		Short:   "Starts a new CoreOS instance",
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) != 0 {
				return fmt.Errorf("Incorrect usage. " +
					"This command doesn't accept any arguments.")
			}
			vipre.BindPFlags(cmd.Flags())

			return SessionContext.allowedToRun()
		},
		RunE: runCommand,
	}
)

func runCommand(cmd *cobra.Command, args []string) error {
	return bootVM(vipre)
}

func bootVM(vipre *viper.Viper) (err error) {
	vm := &VMInfo{}
	c := &exec.Cmd{}

	if vm.Channel, vm.Version, err = lookupImage(
		normalizeChannelName(vipre.GetString("channel")),
		normalizeVersion(vipre.GetString("version")), false); err != nil {
		return
	}
	if err = vm.validateNameAndUUID(vipre.GetString("name"),
		vipre.GetString("uuid")); err != nil {
		return
	}

	vm.Detached = vipre.GetBool("detached")
	vm.Cpus = vipre.GetInt("cpus")
	vm.Extra = vipre.GetString("extra")
	vm.SSHkey = vipre.GetString("sshkey")
	vm.Root, vm.Pid = -1, -1

	if err = vm.xhyveCheck(vipre.GetString("xhyve")); err != nil {
		return
	}

	vm.validateRAM(vipre.GetInt("memory"))

	if err = vm.validateCDROM(vipre.GetString("cdrom")); err != nil {
		return
	}

	if err = vm.validateVolumes([]string{vipre.GetString("root")},
		true); err != nil {
		return
	}
	if err = vm.validateVolumes(pSlice(vipre.GetStringSlice("volume")),
		false); err != nil {
		return
	}

	vm.Ethernet = append(vm.Ethernet, NetworkInterface{Type: Raw})
	if err = vm.addTAPinterface(vipre.GetString("tap")); err != nil {
		return
	}
	if err = vm.validateCloudConfig(
		vipre.GetString("cloud_config")); err != nil {
		return
	}

	if vm.InternalSSHprivKey,
		vm.InternalSSHauthKey, err = sshKeyGen(); err != nil {
		return fmt.Errorf("%v (%v)",
			"Aborting: unable to generate internal SSH key pair (!)", err)
	}

	rundir := filepath.Join(SessionContext.runDir, vm.UUID)
	if err = os.RemoveAll(rundir); err != nil {
		return
	}
	if err = os.MkdirAll(rundir, 0755); err != nil {
		return
	}

	usersDir := &etcExports{}
	usersDir.share()

	fmt.Println("\nbooting ...")
	if c, err = vm.assembleBootPayload(); err != nil {
		return
	}
	vm.CreatedAt = time.Now()
	// saving now, in advance, without Pid to ensure {name,UUID,volumes}
	// atomicity
	if err = vm.storeConfig(); err != nil {
		return
	}
	savePid := func() {
		defer func() { recover() }()
		for _ = range time.Tick(1 * time.Second) {
			vm.Pid = c.Process.Pid
			vm.storeConfig()
			if vm.Detached && err == nil {
				log.Println("started VM in background with PID", c.Process.Pid)
			}
			break
		}
	}

	// FIXME save bootlog
	if !vm.Detached {
		go savePid()
		c.Stdout, c.Stdin, c.Stderr = os.Stdout, os.Stdin, os.Stderr
		if err = c.Run(); err != nil && !strings.HasSuffix(err.Error(),
			"exit status 2") {
			return err
		}
		return nil
	}
	defer savePid()

	if err = c.Start(); err != nil {
		return fmt.Errorf("Aborting: unable to start in background. (%v)", err)
	}
	// usersDir.unshare()
	return
}

func runFlagsDefaults(setFlag *pflag.FlagSet) {
	setFlag.String("channel", "alpha", "CoreOS channel")
	setFlag.String("version", "latest", "CoreOS version")
	setFlag.String("uuid", "random", "VM's UUID")
	setFlag.Int("memory", 1024, "VM's RAM")
	setFlag.Int("cpus", 1, "VM's vCPUS")
	setFlag.String("cloud_config", "",
		"cloud-config file location (either URL or local path)")
	setFlag.String("sshkey", "", "VM's default ssh key")
	setFlag.String("xhyve", "/usr/local/bin/xhyve", "xhyve binary to use")

	if SessionContext.debug {
		setFlag.String("extra", "", "additional arguments to xhyve hypervisor")
	}

	setFlag.String("root", "", "append a (persistent) root volume to VM")
	setFlag.String("cdrom", "", "append an CDROM (.iso) to VM")
	setFlag.StringSlice("volume", nil, "append disk volumes to VM")
	setFlag.String("tap", "", "append tap interface to VM")
	setFlag.BoolP("detached", "d", false,
		"starts the VM in detached (background) mode")
	setFlag.StringP("name", "n", "", "names the VM. (the default is the uuid)")
}

func init() {
	runFlagsDefaults(runCmd.Flags())
	RootCmd.AddCommand(runCmd)
}

type etcExports struct {
	restart, shared    bool
	exports, signature string
	buf                []byte
}

func (f *etcExports) check() {
	f.exports = "/etc/exports"
	var err error

	if f.buf, err = ioutil.ReadFile(f.exports); err != nil {
		log.Fatalln(err)
	}
	f.signature = fmt.Sprintf("/Users %s -alldirs -mapall=%s:%s",
		"-network 192.168.64.0 -mask 255.255.255.0",
		SessionContext.uid, SessionContext.gid)
	f.restart, f.shared = false, false
	lines := strings.Split(string(f.buf), "\n")

	for _, lc := range lines {
		if lc == f.signature {
			f.shared = true
			break
		}
	}
}

func (f *etcExports) reload() {
	if err := exec.Command("nfsd", "restart").Run(); err != nil {
		log.Fatalln("unable to restart NFS...", err)
	}
}

func (f *etcExports) share() {
	f.check()
	if !f.shared {
		ioutil.WriteFile(f.exports,
			append(f.buf, append([]byte("\n"),
				append([]byte(f.signature), []byte("\n")...)...)...),
			os.ModeAppend)
		f.reload()
	}
}

func (f *etcExports) unshare() {
	f.check()
	if f.shared {
		ioutil.WriteFile(f.exports, bytes.Replace(f.buf,
			append(append([]byte("\n"), []byte(f.signature)...),
				[]byte("\n")...), []byte(""), -1), os.ModeAppend)
		f.reload()
	}
}

func (vm *VMInfo) storeConfig() (err error) {
	rundir := filepath.Join(SessionContext.runDir, vm.UUID)
	cfg, _ := json.MarshalIndent(vm, "", "    ")

	if SessionContext.debug {
		fmt.Println(string(cfg))
	}

	if err = ioutil.WriteFile(fmt.Sprintf("%s/config", rundir),
		[]byte(cfg), 0644); err != nil {
		return
	}

	return normalizeOnDiskPermissions(rundir)
}

func (vm *VMInfo) assembleBootPayload() (cmd *exec.Cmd, err error) {
	var (
		cmdline = fmt.Sprintf("%s %s %s %s %s %s=\"%s\"",
			"earlyprintk=serial", "console=ttyS0", "coreos.autologin",
			"localuser="+SessionContext.username, "uuid="+vm.UUID,
			"sshkey_internal", strings.TrimSpace(vm.InternalSSHauthKey))
		prefix  = "coreos_production_pxe"
		vmlinuz = fmt.Sprintf("%s/%s/%s/%s.vmlinuz",
			SessionContext.imageDir, vm.Channel, vm.Version, prefix)
		initrd = fmt.Sprintf("%s/%s/%s/%s_image.cpio.gz",
			SessionContext.imageDir, vm.Channel, vm.Version, prefix)
		instr = []string{
			"-s", "0:0,hostbridge",
			"-l", "com1,stdio",
			"-s", "31,lpc",
			"-U", vm.UUID,
			"-m", fmt.Sprintf("%vM", vm.Memory),
			"-c", fmt.Sprintf("%v", vm.Cpus),
			"-A",
		}
		cc []byte
	)

	if vm.SSHkey != "" {
		cmdline = fmt.Sprintf("%s sshkey=\"%s\"", cmdline, vm.SSHkey)
	}

	if vm.Root != -1 {
		cmdline = fmt.Sprintf("%s root=/dev/vd%s", cmdline, string(vm.Root+'a'))
	}

	if vm.CloudConfig != "" {
		if vm.CClocation == Local {
			cc, err = ioutil.ReadFile(vm.CloudConfig)
			if err = ioutil.WriteFile(
				fmt.Sprintf("%s/%s/cloud-config.local",
					SessionContext.runDir, vm.UUID),
				cc, 0644); err != nil {
				return
			}
		} else {
			cmdline = fmt.Sprintf("%s cloud-config-url=%s",
				cmdline, vm.CloudConfig)
		}
	}

	if vm.Extra != "" {
		instr = append(instr, vm.Extra)
	}

	for v, vv := range vm.Ethernet {
		if vv.Type == Tap {
			instr = append(instr, "-s",
				fmt.Sprintf("2:%d,virtio-tap,%v", v, vv.Path))
		} else {
			instr = append(instr, "-s", fmt.Sprintf("2:%d,virtio-net", v))
		}
	}

	for _, v := range vm.Storage.CDDrives {
		instr = append(instr, "-s", fmt.Sprintf("3:%d,ahci-cd,%s",
			v.Slot, v.Path))
	}

	for _, v := range vm.Storage.HardDrives {
		instr = append(instr, "-s", fmt.Sprintf("4:%d,virtio-blk,%s",
			v.Slot, v.Path))
	}

	return exec.Command(vm.Xhyve, append(instr, "-f",
		fmt.Sprintf("kexec,%s,%s,%s", vmlinuz, initrd, cmdline))...), err
}

func (vm *VMInfo) atomic() (err error) {
	if _, err = vmInfo(vm.Name); err == nil {
		if vm.Name == vm.UUID {
			return fmt.Errorf("%s %s (%s)\n", "Aborting.",
				"Another VM is running with same UUID.", vm.UUID)
		}
		return fmt.Errorf("%s %s (%s)\n", "Aborting.",
			"Another VM is running with same name.", vm.Name)
	}
	return nil
}

func (vm *VMInfo) xhyveCheck(xhyve string) (err error) {
	vm.Xhyve = xhyve
	_, err = exec.LookPath(xhyve)
	return
}

func (vm *VMInfo) validateNameAndUUID(name, xxid string) (err error) {
	if xxid == "random" {
		vm.UUID = uuid.NewV4().String()
	} else {
		if _, err := uuid.FromString(xxid); err != nil {
			log.Printf("%s not a valid UUID as it doesn't follow RFC 4122. %s",
				xxid, "    using a randomly generated one")
			vm.UUID = uuid.NewV4().String()
		} else {
			vm.UUID = xxid
		}
	}
	if name == "" {
		vm.Name = vm.UUID
	} else {
		vm.Name = name
	}
	return vm.atomic()
}

func (vm *VMInfo) validateRAM(ram int) {
	if ram < 1024 {
		fmt.Printf(" '%v' not a reasonable memory value. %s", ram,
			"Using '1024', the default")
		ram = 1024
	} else if ram > 3072 {
		fmt.Printf(" '%v' not a reasonable memory value. %s %s", ram,
			"as presently xhyve only supports VMs with up to 3GB of RAM.",
			"setting it to '3072'")
		ram = 3072
	}
	vm.Memory = ram
}

func (vm *VMInfo) validateCloudConfig(config string) (err error) {
	if len(config) == 0 {
		return
	}

	var response *http.Response
	if response, err = http.Get(config); response != nil {
		response.Body.Close()
	}
	vm.CloudConfig = config
	if err == nil && response.StatusCode == 200 {
		vm.CClocation = Remote
		return
	}
	if _, err = os.Stat(config); err != nil {
		return
	}
	vm.CloudConfig = filepath.Join(SessionContext.pwd, config)
	vm.CClocation = Local
	return
}

func (vm *VMInfo) validateCDROM(path string) (err error) {
	if path == "" {
		return
	}
	var abs string
	if !strings.HasSuffix(path, ".iso") {
		return fmt.Errorf("Aborting: --cdrom payload MUST end in '.iso'"+
			" ('%s' doesn't)", path)
	}
	if _, err = os.Stat(path); err != nil {
		return err
	}
	if abs, err = filepath.Abs(path); err != nil {
		return
	}
	vm.Storage.CDDrives = make(map[string]StorageDevice, 0)
	vm.Storage.CDDrives["0"] = StorageDevice{
		Type: CDROM, Slot: 0, Path: abs,
	}
	return
}

func (vm *VMInfo) addTAPinterface(tap string) (err error) {
	if tap == "" {
		return
	}
	var dir, dev string
	if dir = filepath.Dir(tap); !strings.HasPrefix(dir, "/dev") {
		return fmt.Errorf("Aborting: '%v' not a valid tap device...", tap)
	}
	if dev = filepath.Base(tap); !strings.HasPrefix(dev, "tap") {
		return fmt.Errorf("Aborting: '%v' not a valid tap device...", tap)
	}
	if _, err = os.Stat(tap); err != nil {
		return
	}
	vm.Ethernet = append(vm.Ethernet, NetworkInterface{
		Type: Tap, Path: dev,
	})
	return
}

func (vm *VMInfo) validateVolumes(volumes []string, root bool) (err error) {
	var abs string
	for _, j := range volumes {
		if j != "" {
			if !strings.HasSuffix(j, ".img") {
				return fmt.Errorf("Aborting: --volume payload MUST end"+
					" in '.img' ('%s' doesn't)", j)
			}
			if _, err = os.Stat(j); err != nil {
				return
			}
			if abs, err = filepath.Abs(j); err != nil {
				return
			}
			// check atomicity
			var up []VMInfo
			if up, err = allRunningInstances(); err != nil {
				return
			}
			for _, d := range up {
				for _, vv := range d.Storage.HardDrives {
					if abs == vv.Path {
						return fmt.Errorf("Aborting: %s %s (%s)", abs,
							"already being used as a volume by another VM.",
							vv.Path)
					}
				}
			}

			if vm.Storage.HardDrives == nil {
				vm.Storage.HardDrives = make(map[string]StorageDevice, 0)
			}

			slot := len(vm.Storage.HardDrives)
			for _, z := range vm.Storage.HardDrives {
				if z.Path == abs {
					return fmt.Errorf("Aborting: attempting to set '%v' "+
						"as base of multiple volumes", j)
				}
			}
			vm.Storage.HardDrives[strconv.Itoa(slot)] = StorageDevice{
				Type: HDD, Slot: slot, Path: abs,
			}
			if root {
				vm.Root = slot
			}
		}
	}
	return
}
