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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var (
	psCmd = &cobra.Command{
		Use:     "ps",
		Aliases: []string{"status"},
		Short:   "Lists running CoreOS instances",
		PreRunE: defaultPreRunE,
		RunE:    psCommand,
	}
	queryCmd = &cobra.Command{
		Use:     "query",
		Aliases: []string{"q"},
		Short:   "Display information about the running CoreOS instances",
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			engine.rawArgs.BindPFlags(cmd.Flags())
			return nil
		},
		RunE: queryCommand,
	}
)

func queryCommand(cmd *cobra.Command, args []string) (err error) {
	var (
		pp                []byte
		running, selected []VMInfo
		vm                VMInfo
		tabP              = func(selected []VMInfo) {
			w := new(tabwriter.Writer)
			w.Init(os.Stdout, 5, 0, 1, ' ', 0)
			fmt.Fprintf(w, "name\tchannel/version\tip\tcpu(s)\tram\tuuid\tpid"+
				"\tuptime\tvols\n")
			for _, vm = range selected {
				fmt.Fprintf(w, "%v\t%v/%v\t%v\t%v\t%v\t%v\t%v\t%v\t%v\n",
					vm.Name, vm.Channel, vm.Version, vm.PublicIP, vm.Cpus,
					vm.Memory, vm.UUID, vm.Pid, time.Now().Sub(vm.CreatedAt),
					len(vm.Storage.HardDrives))
			}
			w.Flush()
		}
	)
	if running, err = allRunningInstances(); err != nil {
		return
	}
	if len(args) == 0 {
		if engine.rawArgs.GetBool("json") {
			if pp, err = json.MarshalIndent(running, "", "    "); err == nil {
				fmt.Println(string(pp))
			}
			return
		}
		if engine.rawArgs.GetBool("all") {
			tabP(running)
			return
		}
		for _, vm := range running {
			fmt.Println(vm.Name)
		}
		return
	}

	for _, target := range args {
		if vm, err = vmInfo(target); err != nil {
			return
		}
		selected = append(selected, vm)
	}
	if engine.rawArgs.GetBool("json") {
		if pp, err = json.MarshalIndent(selected, "", "    "); err == nil {
			fmt.Println(string(pp))
		}
		return
	}
	if engine.rawArgs.GetBool("all") {
		tabP(selected)
		return
	}
	for _, vm := range selected {
		fmt.Println(vm.Name)
	}
	return
}

func psCommand(cmd *cobra.Command, args []string) (err error) {
	var (
		pp      []byte
		running []VMInfo
	)

	if running, err = allRunningInstances(); err != nil {
		return
	}
	if engine.rawArgs.GetBool("json") {
		if pp, err = json.MarshalIndent(running, "", "    "); err == nil {
			fmt.Println(string(pp))
		}
		return
	}
	totalV, totalM, totalC := len(running), 0, 0
	for _, vm := range running {
		totalC, totalM = totalC+vm.Cpus, totalM+vm.Memory
	}
	log.Printf("found %v running VMs, summing %v vCPUs and %vMB in use.\n",
		totalV, totalC, totalM)
	for _, vm := range running {
		vm.pp(engine.rawArgs.GetBool("all"))
	}
	return
}

func allRunningInstances() (alive []VMInfo, err error) {
	var ls []os.FileInfo

	if ls, err = ioutil.ReadDir(engine.runDir); err != nil {
		return
	}
	for _, d := range ls {
		if r, e := runningConfig(d.Name()); e == nil {
			alive = append(alive, r)
		}
	}
	return
}

func (vm *VMInfo) pp(extended bool) {
	fmt.Printf("- %v, %v/%v, PID %v (detached=%v), up %v\n",
		vm.Name, vm.Channel, vm.Version, vm.Pid, vm.Detached,
		time.Now().Sub(vm.CreatedAt))
	fmt.Printf("  - %v vCPU(s), %v RAM\n", vm.Cpus, vm.Memory)
	if vm.CloudConfig != "" {
		fmt.Printf("  - cloud-config: %v\n", vm.CloudConfig)
	}
	fmt.Println("  - Network Interfaces:")
	fmt.Printf("    - eth0 (public interface) %v\n", vm.PublicIP)
	if len(vm.Ethernet) > 1 {
		fmt.Printf("    - eth1 (private interface/%v on host)\n", vm.Ethernet[1].Path)
	}
	vm.Storage.pp(vm.Root)
	if extended {
		fmt.Printf("  - UUID: %v\n", vm.UUID)
		if vm.SSHkey != "" {
			fmt.Printf("  - ssh key: %v\n", vm.SSHkey)
		}
		if vm.Extra != "" {
			fmt.Printf("  - custom args to xhyve: %v\n", vm.Extra)
		}
	}
}

func (volumes *storageAssets) pp(root int) {
	if len(volumes.CDDrives)+len(volumes.HardDrives) > 0 {
		fmt.Println("  - Volumes:")
		for a, b := range volumes.CDDrives {
			fmt.Printf("    - /dev/cdrom%v (%s)\n", a, b.Path)
		}
		for a, b := range volumes.HardDrives {
			i, _ := strconv.Atoi(a)
			if i != root {
				fmt.Printf("    - /dev/vd%v (%s)\n", string(i+'a'), b.Path)
			} else {
				fmt.Printf("    - /,/dev/vd%v (%s)\n", string(i+'a'), b.Path)
			}
		}
	}
}

func runningConfig(uuid string) (vm VMInfo, err error) {
	var buf []byte
	if buf, err =
		ioutil.ReadFile(filepath.Join(engine.runDir,
			uuid, "/config")); err != nil {
		return
	}
	json.Unmarshal(buf, &vm)
	if !vm.isActive() {
		return vm, fmt.Errorf("dead")
	}
	return
}

func init() {
	psCmd.Flags().BoolP("all", "a", false,
		"shows extended information about running CoreOS instances")
	psCmd.Flags().BoolP("json", "j", false,
		"outputs in JSON for easy 3rd party integration")
	RootCmd.AddCommand(psCmd)

	queryCmd.Flags().BoolP("json", "j", false,
		"outputs in JSON for easy 3rd party integration")
	queryCmd.Flags().BoolP("all", "a", false,
		"display extended information about a running CoreOS instances")
	RootCmd.AddCommand(queryCmd)
}
