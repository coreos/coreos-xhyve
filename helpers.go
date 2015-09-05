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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mitchellh/go-ps"
	"golang.org/x/crypto/ssh"
)

// (recursively) fix permissions on path
func normalizeOnDiskPermissions(path string) (err error) {
	if !SessionContext.hasPowers {
		return
	}
	u, _ := strconv.Atoi(SessionContext.uid)
	g, _ := strconv.Atoi(SessionContext.gid)

	action := func(p string, _ os.FileInfo, _ error) error {
		return os.Chown(p, u, g)
	}
	return filepath.Walk(path, action)
}

func pSlice(plain []string) []string {
	var sliced []string
	for _, x := range plain {
		strip := strings.Replace(strings.Replace(x, "]", "", -1), "[", "", -1)
		for _, y := range strings.Split(strip, ",") {
			sliced = append(sliced, y)
		}
	}
	return sliced
}

// downloads url to disk and returns its location
func downloadFile(url string) (f string, err error) {
	var (
		tmpDir string
		output *os.File
		r      *http.Response
		n      int64
	)
	if tmpDir, err = ioutil.TempDir("", "coreos"); err != nil {
		return
	}
	defer func() {
		if err != nil {
			if e := os.RemoveAll(tmpDir); e != nil {
				log.Println(e)
			}
		}
	}()
	tmpDir += "/"
	tok := strings.Split(url, "/")
	f = tmpDir + tok[len(tok)-1]
	if SessionContext.debug {
		fmt.Println("    - downloading", url)
	}
	if output, err = os.Create(f); err != nil {
		return url, err
	}
	defer output.Close()
	if r, err = http.Get(url); r != nil {
		defer r.Body.Close()
	} else if err != nil {
		return url, err
	} else if r.StatusCode != 200 {
		return url, err
	}
	if n, err = io.Copy(output, r.Body); err != nil {
		return url, err
	}
	if SessionContext.debug {
		fmt.Println("      -", n, "bytes downloaded.")
	}
	return
}

// sshKeyGen creates a one-time ssh public and private key pair
func sshKeyGen() (string, string, error) {
	secret, err := rsa.GenerateKey(rand.Reader, 2014)
	if err != nil {
		return "", "", err
	}

	secretDer := x509.MarshalPKCS1PrivateKey(secret)
	secretBlk := pem.Block{
		Type: "RSA PRIVATE KEY", Headers: nil, Bytes: secretDer,
	}

	privateKey := string(pem.EncodeToMemory(&secretBlk))

	public, _ := ssh.NewPublicKey(&secret.PublicKey)
	publicFormated := string(ssh.MarshalAuthorizedKey(public))

	return privateKey, publicFormated, nil
}

func (session *sessionInfo) init() (err error) {
	var (
		caller *user.User
		usr    string
	)

	if uid := os.Geteuid(); uid == 0 {
		if usr = os.Getenv("SUDO_USER"); usr == "" {
			return fmt.Errorf("Do not run this as 'root' user," +
				"but as a regular user via 'sudo'")
		}
		if caller, err = user.Lookup(usr); err != nil {
			return
		}
		session.hasPowers = true
	} else {
		session.hasPowers = false
		if caller, err = user.Current(); err != nil {
			return
		}
	}
	session.debug = vipre.GetBool("debug")

	session.configDir = filepath.Join(caller.HomeDir, "/.coreos/")
	session.imageDir = filepath.Join(session.configDir, "/images/")
	session.runDir = filepath.Join(session.configDir, "/running/")

	session.uid, session.gid = caller.Uid, caller.Gid
	session.username = caller.Username

	if session.pwd, err = os.Getwd(); err != nil {
		return
	}

	for _, i := range DefaultChannels {
		if err =
			os.MkdirAll(filepath.Join(session.imageDir, i), 0755); err != nil {
			return
		}
	}

	if err = os.MkdirAll(session.runDir, 0755); err != nil {
		return
	}
	return normalizeOnDiskPermissions(session.configDir)
}

func (session *sessionInfo) allowedToRun() (err error) {
	if !session.hasPowers {
		return fmt.Errorf("not enough previleges to start or halt VMs." +
			" use 'sudo'")
	}
	return
}

func (vm *VMInfo) sshPre() (instr []string, tmpDir string, err error) {
	var secretF string

	if len(vm.PublicIP) == 0 {
		return instr, tmpDir,
			fmt.Errorf("oops, no IP address found for %v", vm.Name)
	}

	if SessionContext.debug {
		fmt.Println("attaching to", vm.Name, vm.PublicIP)
	}

	if tmpDir, err = ioutil.TempDir("", ""); err != nil {
		return
	}

	secretF = filepath.Join(tmpDir, "secret")

	if err = ioutil.WriteFile(secretF,
		[]byte(vm.InternalSSHprivKey), 0600); err != nil {
		return
	}

	instr = []string{fmt.Sprintf("core@%s", vm.PublicIP),
		"-i", secretF, "-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
	}

	return
}
func (vm *VMInfo) sshShell() (err error) {
	var instr []string
	var tmpDir string

	if instr, tmpDir, err = vm.sshPre(); err != nil {
		return
	}

	defer func() {
		if e := os.RemoveAll(tmpDir); e != nil {
			log.Println(e)
		}
	}()

	c := exec.Command("ssh", instr...)
	c.Stdout, c.Stdin, c.Stderr = os.Stdout, os.Stdin, os.Stderr

	if err = c.Run(); err != nil &&
		!strings.Contains(err.Error(), "exit status 255") {
		return err
	}
	return nil
}

func (vm *VMInfo) sshRunCommand(args []string) (out string, err error) {
	var instr []string
	var tmpDir string
	var o []byte

	if instr, tmpDir, err = vm.sshPre(); err != nil {
		return
	}

	defer func() {
		if e := os.RemoveAll(tmpDir); e != nil {
			log.Println(e)
		}
	}()

	instr = append(instr, args...)

	if o, err = exec.Command("ssh", instr...).CombinedOutput(); err != nil &&
		!strings.Contains(err.Error(), "exit status 255") {
		return string(o), err
	}
	return string(o), nil
}

func normalizeChannelName(channel string) string {
	for _, b := range DefaultChannels {
		if b == channel {
			return channel
		}
	}
	log.Printf("'%s' is not a recognized CoreOS image channel. %s",
		channel, "Using default ('alpha').")
	return "alpha"
}

func (vm *VMInfo) isActive() bool {
	if p, _ := ps.FindProcess(vm.Pid); p == nil ||
		!strings.HasPrefix(p.Executable(), "xhyve") {
		return false
	}
	return true
}
