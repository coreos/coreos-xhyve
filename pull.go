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
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/blang/semver"
	"github.com/codeskyblue/go-sh"
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

func findLatestUpstream(channel, version string) (v string, err error) {
	var (
		upstream = fmt.Sprintf("http://%s.%s/%s",
			channel, "release.core-os.net", "amd64-usr/current/version.txt")
		signature = "COREOS_VERSION="
		response  *http.Response
		s         *bufio.Scanner
	)
	if response, err = http.Get(upstream); err != nil {
		// we're probably offline
		return version, err
	}
	if response != nil {
		defer response.Body.Close()
	}

	s = bufio.NewScanner(response.Body)
	s.Split(bufio.ScanLines)
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, signature) {
			version = strings.TrimPrefix(line, signature)
			return version, err
		}
	}
	// shouldn 't happen ever. will be treated as if offline'
	return version, fmt.Errorf("version not found parsing %s (!)", upstream)
}

func lookupImage(channel, version string, override bool) (a, b string, err error) {
	var (
		isLocal bool
		ll      map[string]semver.Versions
		l       semver.Versions
	)
	if ll, err = localImages(); err != nil {
		return
	}
	l = ll[channel]

	if SessionContext.debug {
		fmt.Printf("checking CoreOS %s/%s\n", channel, version)
	}
	if version == "latest" {
		if version, err = findLatestUpstream(channel, version); err != nil {
			// as we're probably offline
			if len(l) == 0 {
				return channel, version,
					fmt.Errorf("offline and not a single locally image"+
						"available for '%s' channel", channel)
			}
			version = l[l.Len()-1].String()
		}
	}
	for _, i := range l {
		if version == i.String() {
			isLocal = true
			break
		}
	}
	if isLocal && !override {
		if SessionContext.debug {
			fmt.Println("    -", version, "already downloaded.")
		}
		return channel, version, err
	}

	return downloadAndVerify(channel, version)
}

func downloadAndVerify(channel, version string) (a, b string, err error) {
	var (
		root = fmt.Sprintf("http://%s.release.core-os.net/amd64-usr/%s/",
			channel, version)
		dest = fmt.Sprintf("%s/%s/%s", SessionContext.imageDir,
			channel, version)
		prefix = "coreos_production_pxe"
		files  = []string{fmt.Sprintf("%s.vmlinuz", prefix),
			fmt.Sprintf("%s_image.cpio.gz", prefix)}
		f, fn, dir, tmpDir, sig string
		out                     []byte
	)

	if err = os.MkdirAll(dest, 0755); err != nil {
		return channel, version, err
	}

	for _, j := range files {
		t := fmt.Sprintf("%s%s", root, j)

		if f, err = downloadFile(t); err != nil {
			return channel, version, err
		}
		if sig, err = downloadFile(fmt.Sprintf("%s.sig", t)); err != nil {
			return channel, version, err
		}

		dir, fn = filepath.Dir(f), filepath.Base(f)

		if tmpDir, err = ioutil.TempDir("", ""); err != nil {
			return channel, version, err
		}
		defer func() {
			if e := os.RemoveAll(tmpDir); e != nil {
				log.Println(e)
			}
			if e := os.RemoveAll(dir); e != nil {
				log.Println(e)
			}
		}()

		if _, err = exec.LookPath("gpg"); err != nil {
			log.Println("'gpg' not found in PATH.",
				"Unable to verify downloaded image's autenticity.")
		} else {
			verify := sh.NewSession()

			verify.SetEnv("GNUPGHOME", tmpDir)
			verify.SetEnv("GPG_LONG_ID", GPGLongID)
			verify.SetEnv("GPG_KEY", GPGKey)
			verify.ShowCMD = false

			verify.Command("gpg", "--batch", "--quiet",
				"--import").SetInput(GPGKey).CombinedOutput()
			out, err = verify.Command("gpg", "--batch", "--trusted-key",
				GPGLongID, "--verify", sig, f).CombinedOutput()
			legit := fmt.Sprintf("%s %s",
				"Good signature from \"CoreOS Buildbot",
				"(Offical Builds) <buildbot@coreos.com>\" [ultimate]")
			if err != nil || !strings.Contains(string(out), legit) {
				return channel, version,
					fmt.Errorf("gpg key verification failed for %v", t)
			}
		}
		if strings.HasSuffix(t, "cpio.gz") {
			oemdir := filepath.Join(dir, "./usr/share/oem/")
			oembindir := filepath.Join(oemdir, "./bin/")
			if err = os.MkdirAll(oembindir, 0755); err != nil {
				return channel, version, err
			}
			if err := ioutil.WriteFile(filepath.Join(oemdir,
				"cloud-config.yml"), []byte(CoreOEMsetup), 0644); err != nil {
				return channel, version, err
			}
			if err := ioutil.WriteFile(filepath.Join(oembindir,
				"coreos-setup-environment"),
				[]byte(CoreOEMsetupEnv), 0755); err != nil {
				return channel, version, err
			}

			oem := sh.NewSession()
			oem.SetDir(dir)
			if out, err = oem.Command("gzip",
				"-dc", fn).Command("cpio",
				"-idv").CombinedOutput(); err != nil {
				return channel, version, err
			}
			if out, err = oem.Command("find",
				"usr", "etc", "usr.squashfs").Command("cpio",
				"-oz", "-H", "newc", "-O", f).CombinedOutput(); err != nil {
				return channel, version, err
			}
		}
		if err = os.Rename(f, fmt.Sprintf("%s/%s", dest, fn)); err != nil {
			return channel, version, err
		}
	}
	return channel, version, normalizeOnDiskPermissions(dest)
}
