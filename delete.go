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
	"path/filepath"

	"github.com/blang/semver"
	"github.com/spf13/cobra"
)

var (
	rmCmd = &cobra.Command{
		Use:     "rm",
		Aliases: []string{"rmi"},
		Short:   "Removes one or more CoreOS images from local fs",
		PreRunE: defaultPreRunE,
		RunE:    rmCommand,
	}
)

func defaultPreRunE(cmd *cobra.Command, args []string) (err error) {
	if len(args) != 0 {
		return fmt.Errorf("Incorrect usage. " +
			"This command doesn't accept any arguments.")
	}
	engine.rawArgs.BindPFlags(cmd.Flags())
	return err
}

func rmCommand(cmd *cobra.Command, args []string) (err error) {
	var (
		channel = normalizeChannelName(engine.rawArgs.GetString("channel"))
		version = normalizeVersion(engine.rawArgs.GetString("version"))
		ll      map[string]semver.Versions
		l       semver.Versions
	)

	if ll, err = localImages(); err != nil {
		return err
	}
	l = ll[channel]

	if engine.rawArgs.GetBool("old") {
		for _, v := range l[0 : l.Len()-1] {
			if err = os.RemoveAll(filepath.Join(engine.imageDir,
				channel, "/", v.String())); err != nil {
				return
			}
			log.Printf("removed %s/%s\n", channel, version)
		}
		return
	}

	if version == "latest" {
		if l.Len() > 0 {
			version = l[l.Len()-1].String()
		} else {
			log.Println("nothing to delete")
			return
		}
	}

	target := filepath.Join(engine.imageDir, channel, "/", version)
	if _, err = os.Stat(target); err != nil {
		log.Printf("%s/%s not found\n", channel, version)
		return nil
	}
	if err = os.RemoveAll(target); err == nil {
		log.Printf("removed %s/%s\n", channel, version)
	}
	return
}

func init() {
	rmCmd.Flags().String("channel", "alpha", "CoreOS channel")
	rmCmd.Flags().String("version", "latest", "CoreOS version")
	rmCmd.Flags().Bool("old", false, "removes outdated images")
	RootCmd.AddCommand(rmCmd)
}
