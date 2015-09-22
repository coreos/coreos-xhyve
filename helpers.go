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
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/blang/semver"
	"github.com/mitchellh/go-ps"
	"github.com/rakyll/pb"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/clearsign"
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

func downloadAndVerify(channel,
	version string) (l map[string]string, err error) {
	var (
		prefix = "coreos_production_pxe"
		root   = fmt.Sprintf("http://%s.release.core-os.net/amd64-usr/%s/",
			channel, version)
		files = []string{fmt.Sprintf("%s.vmlinuz", prefix),
			fmt.Sprintf("%s_image.cpio.gz", prefix)}
		signature = fmt.Sprintf("%s%s%s",
			root, prefix, "_image.cpio.gz.DIGESTS.asc")
		token                                     []string
		tmpDir, digestTxt, fileName, bzHashSHA512 string
		output                                    *os.File
		digestRaw, longIDdecoded                  []byte
		r, digest                                 *http.Response
		longIDdecodedInt                          uint64
		keyring                                   openpgp.EntityList
		check                                     *openpgp.Entity
		messageClear                              *clearsign.Block
		messageClearRdr                           *bytes.Reader
		re                                        = regexp.MustCompile(
			`(?m)(?P<method>(SHA1|SHA512)) HASH(?:\r?)\n(?P<hash>` +
				`.[^\s]*)\s*(?P<file>[\w\d_\.]*)`)
		keymap   = make(map[string]int)
		location = make(map[string]string)
	)

	log.Printf("downloading and verifying %s/%v\n", channel, version)
	for _, target := range files {
		url := fmt.Sprintf("%s%s", root, target)

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
		token = strings.Split(url, "/")
		fileName = token[len(token)-1]
		pack := filepath.Join(tmpDir, "/", fileName)
		if _, err = http.Head(url); err != nil {
			return
		}
		if digest, err = http.Get(signature); err != nil {
			return
		}
		defer digest.Body.Close()
		switch digest.StatusCode {
		case http.StatusOK, http.StatusNoContent:
		default:
			return l, fmt.Errorf("failed fetching %s: HTTP status: %s",
				signature, digest.Status)
		}
		if digestRaw, err = ioutil.ReadAll(digest.Body); err != nil {
			return
		}
		if longIDdecoded, err = hex.DecodeString(GPGLongID); err != nil {
			return
		}
		longIDdecodedInt = binary.BigEndian.Uint64(longIDdecoded)
		if SessionContext.debug {
			fmt.Printf("Trusted hex key id %s is decimal %d\n",
				GPGLongID, longIDdecoded)
		}
		if keyring, err = openpgp.ReadArmoredKeyRing(
			bytes.NewBufferString(GPGKey)); err != nil {
			return
		}
		messageClear, _ = clearsign.Decode(digestRaw)
		digestTxt = string(messageClear.Bytes)
		messageClearRdr = bytes.NewReader(messageClear.Bytes)
		if check, err =
			openpgp.CheckDetachedSignature(keyring, messageClearRdr,
				messageClear.ArmoredSignature.Body); err != nil {
			return l, fmt.Errorf("Signature check for DIGESTS failed.")
		}
		if check.PrimaryKey.KeyId == longIDdecodedInt {
			if SessionContext.debug {
				fmt.Printf("Trusted key id %d matches keyid %d\n",
					longIDdecodedInt, longIDdecodedInt)
			}
		}
		if SessionContext.debug {
			fmt.Printf("DIGESTS signature OK. ")
		}

		for index, name := range re.SubexpNames() {
			keymap[name] = index
		}

		matches := re.FindAllStringSubmatch(digestTxt, -1)

		for _, match := range matches {
			if match[keymap["file"]] == fileName {
				if match[keymap["method"]] == "SHA512" {
					bzHashSHA512 = match[keymap["hash"]]
				}
			}
		}

		sha512h := sha512.New()

		if r, err = http.Get(url); err != nil {
			return
		}
		defer r.Body.Close()
		switch r.StatusCode {
		case http.StatusOK, http.StatusNoContent:
		default:
			return l, fmt.Errorf("failed fetching %s: HTTP status: %s",
				signature, r.Status)
		}
		bar := pb.New(int(r.ContentLength)).SetUnits(pb.U_BYTES)
		bar.Start()

		if output, err = os.Create(pack); err != nil {
			return
		}
		defer output.Close()

		writer := io.MultiWriter(sha512h, bar, output)
		io.Copy(writer, r.Body)
		bar.Finish()
		if hex.EncodeToString(sha512h.Sum([]byte{})) != bzHashSHA512 {
			return l, fmt.Errorf("SHA512 hash verification failed for %s",
				fileName)
		}
		log.Printf("SHA512 hash for %s OK\n", fileName)

		location[fileName] = pack
	}
	return location, err
}

// sshKeyGen creates a one-time ssh public and private key pair
func sshKeyGen() (a string, b string, err error) {
	var (
		public ssh.PublicKey
		secret *rsa.PrivateKey
	)

	if secret, err = rsa.GenerateKey(rand.Reader, 2014); err != nil {
		return
	}

	secretDer := x509.MarshalPKCS1PrivateKey(secret)
	secretBlk := pem.Block{
		Type: "RSA PRIVATE KEY", Headers: nil, Bytes: secretDer,
	}
	if public, err = ssh.NewPublicKey(&secret.PublicKey); err != nil {
		return
	}

	return string(pem.EncodeToMemory(&secretBlk)),
		string(ssh.MarshalAuthorizedKey(public)), err
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

func normalizeVersion(version string) string {
	if version == "latest" {
		return version
	}
	if _, err := semver.Parse(version); err != nil {
		log.Printf("'%s' is not in a recognizable CoreOS version format. %s",
			version, "Using default ('latest') instead")
		return "latest"
	}
	return version
}

func (vm *VMInfo) isActive() bool {
	clean := func() {
		staled := filepath.Join(SessionContext.runDir, vm.UUID)
		if SessionContext.debug {
			log.Println("removing staled", staled)
		}

		if e :=
			os.RemoveAll(staled); e != nil {
			log.Println(e)
		}
	}
	if vm.Pid < 0 {
		clean()
		return false
	}
	if p, _ := ps.FindProcess(vm.Pid); p == nil ||
		!strings.HasPrefix(p.Executable(), "xhyve") {
		clean()
		return false
	}
	return true
}
