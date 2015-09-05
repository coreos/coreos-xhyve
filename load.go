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
	"fmt"
	"io/ioutil"
	"reflect"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	loadFCmd = &cobra.Command{
		Use: "load",
		Short: "Loads from an instrumentation file " +
			"(in TOML, JSON or YAML) one or more CoreOS instances",
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) != 1 {
				return fmt.Errorf("Incorrect usage: " +
					"This command requires one argument (a file path)")
			}
			vipre.BindPFlags(cmd.Flags())
			return SessionContext.allowedToRun()
		},
		RunE:    loadCommand,
		Example: `  coreos load profiles/demo.toml`,
	}
)

func loadCommand(cmd *cobra.Command, args []string) (err error) {
	var (
		vmDefs = make(map[string]*viper.Viper)
		f      []byte
		def    = args[0]
		setup  = viper.New()
	)

	if f, err = ioutil.ReadFile(def); err != nil {
		return
	}

	if strings.HasSuffix(def, ".toml") {
		setup.SetConfigType("toml")
	} else if strings.HasSuffix(def, ".json") {
		setup.SetConfigType("json")
	} else if strings.HasSuffix(def, ".yaml") ||
		strings.HasSuffix(def, ".yml") {
		setup.SetConfigType("yaml")
	} else {
		return fmt.Errorf("%s unable to guess format via suffix", def)
	}

	if err = setup.ReadConfig(bytes.NewBuffer(f)); err != nil {
		return
	}

	for name, def := range setup.AllSettings() {
		if reflect.ValueOf(def).Kind() == reflect.Map {
			lf := pflag.NewFlagSet(name, 0)
			runFlagsDefaults(lf)
			vmDefs[name] = viper.New()
			vmDefs[name].BindPFlags(lf)

			for x, xx := range setup.AllSettings() {
				if reflect.ValueOf(x).Kind() != reflect.Map {
					vmDefs[name].Set(x, xx)
				}
			}
			for x, xx := range def.(map[string]interface{}) {
				vmDefs[name].Set(x, xx)
			}
			vmDefs[name].Set("name", name)
			vmDefs[name].Set("detached", true)
		}
	}

	for name, defs := range vmDefs {
		fmt.Println("> booting", name)
		if err = bootVM(defs); err != nil {
			return
		}
	}

	return
}

func init() {
	RootCmd.AddCommand(loadFCmd)
}
