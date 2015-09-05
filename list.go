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
	"os"
	"path/filepath"

	"github.com/blang/semver"
	"github.com/spf13/cobra"
)

var (
	lsCmd = &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "Lists locally available CoreOS images",
		PreRunE: defaultPreRunE,
		RunE:    lsCommand,
	}
)

func lsCommand(cmd *cobra.Command, args []string) (err error) {
	var (
		channels []string
		local    map[string]semver.Versions
	)
	if local, err = localImages(); err != nil {
		return
	}

	if vipre.GetBool("all") {
		channels = DefaultChannels
	} else {
		channels = append(channels, vipre.GetString("channel"))
	}
	if vipre.GetBool("json") {
		var pp []byte
		if len(channels) == 1 {
			if pp, err = json.MarshalIndent(
				local[vipre.GetString("channel")],
				"", "    "); err != nil {
				return
			}
		} else {
			if pp, err = json.MarshalIndent(local, "", "    "); err != nil {
				return
			}
		}
		fmt.Println(string(pp))
		return
	}
	fmt.Println("locally available images")
	for _, i := range channels {
		fmt.Printf("  - %s channel \n", i)
		for _, d := range local[i] {
			fmt.Println("    -", d.String())
		}
	}
	return
}

func init() {
	lsCmd.Flags().String("channel", "alpha", "CoreOS channel")
	lsCmd.Flags().BoolP("all", "a", false, "browses all channels")
	lsCmd.Flags().BoolP("json", "j", false,
		"outputs in JSON for easy 3rd party integration")
	RootCmd.AddCommand(lsCmd)
}

func localImages() (local map[string]semver.Versions, err error) {
	var (
		files   []os.FileInfo
		f       os.FileInfo
		channel string
	)
	local = make(map[string]semver.Versions, 0)
	for _, channel = range DefaultChannels {
		if files, err = ioutil.ReadDir(filepath.Join(SessionContext.imageDir,
			channel)); err != nil {
			return
		}
		var v semver.Versions
		for _, f = range files {
			if f.IsDir() {
				var s semver.Version
				if s, err = semver.Make(f.Name()); err != nil {
					return
				}
				v = append(v, s)
			}
		}
		semver.Sort(v)
		local[channel] = v
	}
	return
}
