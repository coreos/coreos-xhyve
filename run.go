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
	"encoding/base64"
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

	"github.com/TheNewNormal/corectl/uuid2ip"
	"github.com/hooklift/xhyve"
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
	xhyveCmd = &cobra.Command{
		Use:    "xhyve",
		Hidden: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 3 {
				return fmt.Errorf("Incorrect usage. " +
					"This command accepts exactly 3 arguments.")
			}
			return nil
		},
		RunE: xhyveCommand,
	}
)

func runCommand(cmd *cobra.Command, args []string) error {
	return bootVM(vipre)
}

func xhyveCommand(cmd *cobra.Command, args []string) (err error) {
	var (
		a0, a1, a2 string
		strDecode  = func(s string) (string, error) {
			b, e := base64.StdEncoding.DecodeString(s)
			return string(b), e
		}
	)

	if a0, err = strDecode(args[0]); err != nil {
		return err
	}
	if a1, err = strDecode(args[1]); err != nil {
		return err
	}
	if a2, err = strDecode(args[2]); err != nil {
		return err
	}
	return xhyve.Run(append(strings.Split(a0, " "),
		"-f", fmt.Sprintf("%s%v", a1, a2)), make(chan string))
}

func bootVM(vipre *viper.Viper) (err error) {
	var (
		rundir string
		vm, c  = &VMInfo{}, &exec.Cmd{}
	)

	vm.publicIP = make(chan string)

	vm.PreferLocalImages = vipre.GetBool("local")
	if vm.Channel, vm.Version, err =
		lookupImage(normalizeChannelName(vipre.GetString("channel")),
			normalizeVersion(vipre.GetString("version")), false,
			vm.PreferLocalImages); err != nil {
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

	err = vm.validateCloudConfig(vipre.GetString("cloud_config"))
	if err != nil {
		return
	}

	vm.InternalSSHprivKey, vm.InternalSSHauthKey, err = sshKeyGen()
	if err != nil {
		return fmt.Errorf("%v (%v)",
			"Aborting: unable to generate internal SSH key pair (!)", err)
	}

	rundir = filepath.Join(SessionContext.runDir, vm.UUID)
	if err = os.RemoveAll(rundir); err != nil {
		return
	}
	if err = os.MkdirAll(rundir, 0755); err != nil {
		return
	}

	usersDir := &etcExports{}
	usersDir.share()

	if c, err = vm.assembleBootPayload(); err != nil {
		return
	}
	vm.CreatedAt = time.Now()
	// saving now, in advance, without Pid to ensure {name,UUID,volumes}
	// atomicity
	if err = vm.storeConfig(); err != nil {
		return
	}

	go func() {
		select {
		case <-time.After(45 * time.Second):
			log.Println("Unable to grab VM's pid and IP after 15s (!)... " +
				"Aborting")
			return
		case <-time.Tick(100 * time.Millisecond):
			vm.Pid = c.Process.Pid
			select {
			case ip := <-vm.publicIP:
				vm.PublicIP = ip
				vm.storeConfig()
				close(vm.publicIP)
			}
		}
	}()

	defer func() {
		wg.Wait()
		if vm.Detached && err == nil {
			log.Printf("started '%s' in background with IP %v and PID %v\n",
				vm.Name, vm.PublicIP, c.Process.Pid)
		}
	}()

	if !vm.Detached {
		c.Stdout, c.Stdin, c.Stderr = os.Stdout, os.Stdin, os.Stderr
		return c.Run()
	}

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
	setFlag.Int("memory", 1024,
		"VM's RAM, in MB, per instance (1024 < memory < 3072)")
	setFlag.Int("cpus", 1, "VM's vCPUS")
	setFlag.String("cloud_config", "",
		"cloud-config file location (either a remote URL or a local path)")
	setFlag.String("sshkey", "", "VM's default ssh key")
	setFlag.String("root", "", "append a (persistent) root volume to VM")
	setFlag.String("cdrom", "", "append an CDROM (.iso) to VM")
	setFlag.StringSlice("volume", nil, "append disk volumes to VM")
	setFlag.String("tap", "", "append tap interface to VM")
	setFlag.BoolP("detached", "d", false,
		"starts the VM in detached (background) mode")
	setFlag.BoolP("local", "l", false,
		"consumes whatever image is `latest` locally instead of looking "+
			"online unless there's nothing available.")
	setFlag.StringP("name", "n", "",
		"names the VM. (if absent defaults to VM's UUID)")

	// available but hidden...
	setFlag.String("extra", "", "additional arguments to xhyve hypervisor")
	setFlag.MarkHidden("extra")
}

func init() {
	runFlagsDefaults(runCmd.Flags())
	RootCmd.AddCommand(runCmd)
	RootCmd.AddCommand(xhyveCmd)
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
		cmdline = fmt.Sprintf("%s %s %s %s",
			"earlyprintk=serial", "console=ttyS0", "coreos.autologin",
			"uuid="+vm.UUID)
		prefix  = "coreos_production_pxe"
		vmlinuz = fmt.Sprintf("%s/%s/%s/%s.vmlinuz",
			SessionContext.imageDir, vm.Channel, vm.Version, prefix)
		initrd = fmt.Sprintf("%s/%s/%s/%s_image.cpio.gz",
			SessionContext.imageDir, vm.Channel, vm.Version, prefix)
		instr = []string{
			"libxhyve_bug",
			"-s", "0:0,hostbridge",
			"-l", "com1,stdio",
			"-s", "31,lpc",
			"-U", vm.UUID,
			"-m", fmt.Sprintf("%vM", vm.Memory),
			"-c", fmt.Sprintf("%v", vm.Cpus),
			"-A",
		}
		endpoint string
	)

	if vm.SSHkey != "" {
		cmdline = fmt.Sprintf("%s sshkey=\"%s\"", cmdline, vm.SSHkey)
	}

	if vm.Root != -1 {
		cmdline = fmt.Sprintf("%s root=/dev/vd%s", cmdline, string(vm.Root+'a'))
	}

	if endpoint, err = vm.metadataService(); err != nil {
		return
	}
	cmdline = fmt.Sprintf("%s endpoint=%s", cmdline, endpoint)

	if vm.CloudConfig != "" {
		if vm.CClocation == Local {
			cmdline = fmt.Sprintf("%s cloud-config-url=%s",
				cmdline, endpoint+"/cloud-config")
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
			instr = append(instr,
				"-s", fmt.Sprintf("2:%d,virtio-tap,%v", v, vv.Path))
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
	strEncode := func(s string) string {
		return base64.StdEncoding.EncodeToString([]byte(s))
	}
	return exec.Command(os.Args[0], "xhyve",
			strEncode(strings.Join(instr, " ")),
			strEncode(fmt.Sprintf("kexec,%s,%s,", vmlinuz, initrd)),
			strEncode(fmt.Sprintf("%v", cmdline))),
		err
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

func (vm *VMInfo) validateNameAndUUID(name, xxid string) (err error) {
	if xxid == "random" {
		vm.UUID = uuid.NewV4().String()
	} else if _, err = uuid.FromString(xxid); err != nil {
		log.Printf("%s not a valid UUID as it doesn't follow RFC 4122. %s\n",
			xxid, "    using a randomly generated one")
		vm.UUID = uuid.NewV4().String()
	} else {
		vm.UUID = xxid
	}
	for {
		if vm.MacAddress, err = uuid2ip.GuestMACfromUUID(vm.UUID); err != nil {
			if xxid != "random" {
				log.Printf("unable to guess the MAC Address from the provided "+
					"UUID (%s). Using a randomly generated one one\n",	xxid)
			}
			vm.UUID = uuid.NewV4().String()
		} else {
			break
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
		log.Printf("'%v' not a reasonable memory value. %s\n", ram,
			"Using '1024', the default")
		ram = 1024
	} else if ram > 3072 {
		log.Printf("'%v' not a reasonable memory value. %s %s\n", ram,
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
	if err == nil && (response.StatusCode == http.StatusOK ||
		response.StatusCode == http.StatusNoContent) {
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
	// check atomicity
	var up []VMInfo
	if up, err = allRunningInstances(); err != nil {
		return
	}
	for _, d := range up {
		for _, vv := range d.Ethernet {
			if dev == vv.Path {
				return fmt.Errorf("Aborting: %s already being used  "+
					"by another VM (%s)", dev,
					d.Name)
			}
		}
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
			if _, err = os.Stat(j); err != nil {
				return
			}
			if abs, err = filepath.Abs(j); err != nil {
				return
			}
			if !strings.HasSuffix(j, ".img") {
				return fmt.Errorf("Aborting: --volume payload MUST end"+
					" in '.img' ('%s' doesn't)", j)
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
