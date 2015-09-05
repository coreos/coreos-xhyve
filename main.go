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
	"github.com/spf13/viper"
)

var (
	// RootCmd ...
	RootCmd = &cobra.Command{
		Use:   "coreos",
		Short: "CoreOS, on top of OS X and xhyve, made simple.",
		Long: fmt.Sprintf("%s\n%s",
			"CoreOS, on top of OS X and xhyve, made simple.",
			"❯❯❯ http://github.com/coreos/coreos-xhyve"),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			versionCommand(cmd, args)
			return cmd.Usage()
		},
	}
	versionCmd = &cobra.Command{
		Use: "version", Short: "Show the (coreos-xhyve) version information",
		Run: versionCommand,
	}
)

func main() {
	if err := RootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}
func init() {
	// viper & cobra
	vipre = viper.New()
	vipre.SetEnvPrefix("COREOS")
	vipre.AutomaticEnv()

	RootCmd.PersistentFlags().Bool("debug", false,
		"adds extra verbosity, and options, for debugging purposes "+
			"and/or power users")

	RootCmd.SetUsageTemplate(HelpTemplate)
	RootCmd.AddCommand(versionCmd)

	vipre.BindPFlags(RootCmd.PersistentFlags())

	// logger defaults
	log.SetFlags(0)
	log.SetOutput(os.Stderr)
	log.SetPrefix("[coreos] ")

	// remaining defaults / startupChecks
	SessionContext.init()
}

func versionCommand(cmd *cobra.Command, args []string) {
	fmt.Println("coreos version", Version)
}
