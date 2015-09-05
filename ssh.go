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
	"fmt"

	"github.com/spf13/cobra"
)

var (
	sshCmd = &cobra.Command{
		Use:     "ssh",
		Aliases: []string{"attach"},
		Short:   "Attach to or run commands inside a running CoreOS instance",
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			vipre.BindPFlags(cmd.Flags())
			if len(args) < 1 {
				return fmt.Errorf("This command requires either at least " +
					"one argument to work ")
			}
			return
		},
		RunE: sshCommand,
		Example: `  coreos ssh VMid                 // logins into VMid
  coreos ssh VMid "some commands" // runs 'some commands' inside VMid and exits`,
	}
)

func sshCommand(cmd *cobra.Command, args []string) (err error) {
	var out string
	vm := VMInfo{}

	if vm, err = vmInfo(args[0]); err != nil {
		return
	}
	if len(args) == 1 {
		return vm.sshShell()
	}
	if out, err = vm.sshRunCommand(args[1:]); err != nil {
		return
	}
	fmt.Printf(out)
	return
}

func vmInfo(id string) (vm VMInfo, err error) {
	var up []VMInfo
	if up, err = allRunningInstances(); err != nil {
		return
	}
	for _, v := range up {
		if v.Name == id || v.UUID == id {
			return v, err
		}
	}
	return vm, fmt.Errorf("'%s' not found, or dead", id)
}

func init() {
	RootCmd.AddCommand(sshCmd)
}
