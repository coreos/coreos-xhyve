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
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	// XXX
	"github.com/AntonioMeireles/coreos-xhyve/image"

	"github.com/blang/semver"
	"github.com/spf13/cobra"
)

var (
	pullCmd = &cobra.Command{
		Use:     "pull",
		Aliases: []string{"get", "fetch"},
		Short:   "Pulls a CoreOS image from upstream",
		PreRunE: defaultPreRunE,
		RunE:    pullCommand,
	}
)

func pullCommand(cmd *cobra.Command, args []string) (err error) {
	_, _, err = lookupImage(normalizeChannelName(vipre.GetString("channel")),
		normalizeVersion(vipre.GetString("version")), vipre.GetBool("force"))
	return
}

func init() {
	pullCmd.Flags().String("channel", "alpha", "CoreOS channel")
	pullCmd.Flags().String("version", "latest", "CoreOS version")
	pullCmd.Flags().BoolP("force", "f", false, "override local image, if any")
	RootCmd.AddCommand(pullCmd)
}

func findLatestUpstream(channel string) (releaseInfo map[string]string, err error) {
	var (
		upstream = fmt.Sprintf("http://%s.%s/%s", channel,
			"release.core-os.net", "amd64-usr/current/version.txt")
		response *http.Response
		s        *bufio.Scanner
	)
	releaseInfo = make(map[string]string)
	if response, err = http.Get(upstream); err != nil {
		// we're probably offline
		return
	}

	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK, http.StatusNoContent:
	default:
		return releaseInfo, fmt.Errorf("failed fetching %s: HTTP status: %s",
			upstream, response.Status)
	}

	s = bufio.NewScanner(response.Body)
	s.Split(bufio.ScanLines)
	for s.Scan() {
		line := s.Text()
		if eq := strings.Index(line, "="); eq >= 0 {
			if key := strings.TrimSpace(line[:eq]); len(key) > 0 {
				v := ""
				if len(line) > eq {
					v = strings.TrimSpace(line[eq+1:])
				}
				releaseInfo[key] = v
			}
		}
	}
	return
}

func lookupImage(channel, version string, override bool) (a, b string, err error) {
	var (
		isLocal     bool
		ll          map[string]semver.Versions
		l           semver.Versions
		releaseInfo map[string]string
	)

	if ll, err = localImages(); err != nil {
		return channel, version, err
	}
	l = ll[channel]
	if version == "latest" {
		if releaseInfo, err = findLatestUpstream(channel); err != nil {
			// as we're probably offline
			if len(l) == 0 {
				err = fmt.Errorf("offline and not a single locally image"+
					"available for '%s' channel", channel)
				return channel, version, err
			}
			version = l[l.Len()-1].String()
		} else {
			version = releaseInfo["COREOS_VERSION"]
		}
	}
	for _, i := range l {
		if version == i.String() {
			isLocal = true
			break
		}
	}
	if isLocal && !override {
		log.Printf("%s/%s already available or your system\n", channel, version)
		return channel, version, err
	}
	return localize(channel, version)
}

func localize(channel, version string) (a string, b string, err error) {
	var files map[string]string
	destination := fmt.Sprintf("%s/%s/%s", SessionContext.imageDir,
		channel, version)

	if err = os.MkdirAll(destination, 0755); err != nil {
		return channel, version, err
	}
	if files, err = downloadAndVerify(channel, version); err != nil {
		return channel, version, err
	}
	defer func() {
		for _, location := range files {
			if e := os.RemoveAll(filepath.Dir(location)); e != nil {
				log.Println(e)
			}
		}
	}()
	for fn, location := range files {
		// OEMify
		if strings.HasSuffix(fn, "cpio.gz") {
			var (
				i, temp *os.File
				r       *image.Reader
				w       *image.Writer
			)

			if i, err = os.Open(location); err != nil {
				return
			}
			defer i.Close()

			if r, err = image.NewReader(i); err != nil {
				return
			}
			defer r.Close()

			if temp, err = ioutil.TempFile("", "coreos"); err != nil {
				return
			}
			defer temp.Close()

			if w, err = image.NewWriter(temp); err != nil {
				return
			}

			for _, d := range []string{"usr", "usr/share", "usr/share/oem",
				"usr/share/oem/bin"} {
				if err = w.WriteDir(d, 0755); err != nil {
					return
				}
			}

			if err = w.WriteToFile(bytes.NewBufferString(CoreOEMsetupBootstrap),
				"usr/share/oem/cloud-config.yml", 0644); err != nil {
				return
			}

			if err = w.WriteToFile(bytes.NewBufferString(
				strings.Replace(CoreOEMsetup, "@@version@@", version, -1)),
				"usr/share/oem/xhyve.yml", 0644); err != nil {
				return
			}

			if err = w.WriteToFile(bytes.NewBufferString(CoreOEMsetupEnv),
				"usr/share/oem/bin/coreos-setup-environment",
				0755); err != nil {
				return
			}
			defer w.Close()

			if err = image.Copy(w, r); err != nil {
				return
			}
			if err = os.Rename(temp.Name(), location); err != nil {
				return
			}

		}
		if err = os.Rename(location,
			fmt.Sprintf("%s/%s", destination, fn)); err != nil {
			return channel, version, err
		}
	}
	if err = normalizeOnDiskPermissions(destination); err == nil {
		log.Printf("%s/%s ready\n", channel, version)
	}
	return channel, version, err
}
