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
	"log"
	"os"

	"github.com/spf13/cobra"
)

var (
	killCmd = &cobra.Command{
		Use:     "kill",
		Aliases: []string{"stop", "halt"},
		Short:   "Halts one or more running CoreOS instances",
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			vipre.BindPFlags(cmd.Flags())
			if len(args) < 1 && !vipre.GetBool("all") {
				return fmt.Errorf("This command requires either at least " +
					"one argument to work or --all.")
			}
			return SessionContext.allowedToRun()
		},
		RunE: killCommand,
	}
)

func killCommand(cmd *cobra.Command, args []string) (err error) {
	var up []VMInfo
	if up, err = allRunningInstances(); err != nil {
		return
	}
	if vipre.GetBool("all") {
		for _, vm := range up {
			if err = vm.halt(); err != nil {
				return err
			}
		}
		return
	}
	for _, arg := range args {
		for _, vm := range up {
			if vm.Name == arg || vm.UUID == arg {
				if err = vm.halt(); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("%v is already dead or never was up", arg)
			}
		}
	}
	return
}

func (vm *VMInfo) halt() (err error) {
	if _, e := vm.sshRunCommand([]string{"sudo sync;id"}); e != nil {
		// ssh messed up for some reason or target has no IP
		log.Printf("couldn't ssh to %v (%v)...\n", vm.Name, e)
		if p, ee := os.FindProcess(vm.Pid); ee == nil {
			log.Println("hard kill...")
			if eee := p.Kill(); eee != nil {
				return eee
			}
		}
		return nil
	}
	_, err = vm.sshRunCommand([]string{"sudo halt"})
	return
}

func init() {
	killCmd.Flags().BoolP("all", "a", false, "halts all running instances")
	RootCmd.AddCommand(killCmd)
}
