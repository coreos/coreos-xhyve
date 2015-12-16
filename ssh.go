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
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"github.com/rakyll/pb"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	sshCmd = &cobra.Command{
		Use:     "ssh",
		Aliases: []string{"attach"},
		Short:   "Attach to or run commands inside a running CoreOS instance",
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			engine.rawArgs.BindPFlags(cmd.Flags())
			if len(args) < 1 {
				return fmt.Errorf("This command requires at least " +
					"one argument to work ")
			}
			return
		},
		RunE: sshCommand,
		Example: `  corectl ssh VMid                 // logins into VMid
  corectl ssh VMid "some commands" // runs 'some commands' inside VMid and exits`,
	}
	scpCmd = &cobra.Command{
		Use:     "put",
		Aliases: []string{"copy", "cp", "scp"},
		Short:   "copy file to inside VM",
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			engine.rawArgs.BindPFlags(cmd.Flags())
			if len(args) < 2 {
				return fmt.Errorf("This command requires at least " +
					"two argument to work ")
			}
			return
		},
		RunE: scpCommand,
		Example: `  // copies 'filePath' into '/destinationPath' inside VMid
  corectl put filePath VMid:/destinationPath`,
	}
)

func sshCommand(cmd *cobra.Command, args []string) (err error) {
	var sshSession = &sshClient{}
	vm := VMInfo{}

	if vm, err = vmInfo(args[0]); err != nil {
		return
	}

	if sshSession, err = vm.startSSHsession(); err != nil {
		return
	}
	defer sshSession.close()

	if len(args) > 1 {
		return sshSession.executeRemoteCommand(strings.Join(args[1:], " "))
	}
	return sshSession.remoteShell()
}

type sshClient struct {
	session                   *ssh.Session
	conn                      *ssh.Client
	oldState                  *terminal.State
	termWidth, termHeight, fd int
}

func (c *sshClient) close() {
	c.conn.Close()
	c.session.Close()
	terminal.Restore(c.fd, c.oldState)
}

func (vm VMInfo) startSSHsession() (c *sshClient, err error) {
	var secret ssh.Signer
	c = &sshClient{}

	if secret, err = ssh.ParsePrivateKey(
		[]byte(vm.InternalSSHprivKey)); err != nil {
		return
	}

	config := &ssh.ClientConfig{
		User: "core", Auth: []ssh.AuthMethod{
			ssh.PublicKeys(secret),
		},
	}

	//wait a bit for VM's ssh to be available...
	for {
		var e error
		if c.conn, e = ssh.Dial("tcp", vm.PublicIP+":22", config); e == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
		select {
		case <-time.After(time.Second * 5):
			return c, fmt.Errorf("%s unreachable", vm.PublicIP+":22")
		}
	}

	if c.session, err = c.conn.NewSession(); err != nil {
		return c, fmt.Errorf("unable to create session: %s", err)
	}

	c.fd = int(os.Stdin.Fd())
	if c.oldState, err = terminal.MakeRaw(c.fd); err != nil {
		return
	}

	c.session.Stdout, c.session.Stderr, c.session.Stdin =
		os.Stdout, os.Stderr, os.Stdin

	if c.termWidth, c.termHeight, err = terminal.GetSize(c.fd); err != nil {
		return
	}

	modes := ssh.TerminalModes{
		ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400,
	}

	if err = c.session.RequestPty("xterm-256color",
		c.termHeight, c.termWidth, modes); err != nil {
		return c, fmt.Errorf("request for pseudo terminal failed: %s", err)
	}
	return
}

func (c *sshClient) executeRemoteCommand(run string) (err error) {
	if err = c.session.Run(run); err != nil && !strings.HasSuffix(err.Error(),
		"exited without exit status or exit signal") {
		return
	}
	return nil
}

func (c *sshClient) remoteShell() (err error) {
	if err = c.session.Shell(); err != nil {
		return
	}

	if err = c.session.Wait(); err != nil && !strings.HasSuffix(err.Error(),
		"exited without exit status or exit signal") {
		return err
	}
	return nil
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

func (c *sshClient) sCopy(source, destination, target string) (err error) {
	var (
		ftp         *sftp.Client
		src         *os.File
		srcS, destS os.FileInfo
		dest        *sftp.File
		bar         *pb.ProgressBar
	)

	if ftp, err = sftp.NewClient(c.conn); err != nil {
		return
	}
	defer ftp.Close()

	if src, err = os.Open(source); err != nil {
		return
	}
	defer src.Close()
	if srcS, err = os.Stat(source); err != nil {
		return
	}
	if _, err = ftp.ReadDir(filepath.Dir(destination)); err != nil {
		err = fmt.Errorf("unable to upload %v as parent %v "+
			"not in target", source, filepath.Dir(destination))
		return
	}
	if _, err = ftp.ReadDir(destination); err == nil {
		destination = ftp.Join(destination, "/", filepath.Base(source))
	}
	if dest, err = ftp.Create(destination); err != nil {
		return
	}
	defer dest.Close()
	log.Println("uploading '" + source + "' to '" +
		target + ":" + destination + "'")
	bar = pb.New(int(srcS.Size())).SetUnits(pb.U_BYTES)
	bar.Start()
	defer bar.Finish()
	if _, err = io.Copy(dest, src); err != nil {
		return
	}

	if destS, err = ftp.Stat(destination); err != nil {
		return
	}
	if srcS.Size() != destS.Size() {
		err = fmt.Errorf("something went wrong. " +
			"destination file size != from sources'")
	}
	return
}

func scpCommand(cmd *cobra.Command, args []string) (err error) {
	var (
		session, vm                 = &sshClient{}, VMInfo{}
		split                       = strings.Split(args[1], ":")
		source, destination, target = args[0], split[1], split[0]
	)
	if vm, err = vmInfo(target); err != nil {
		return
	}
	if session, err = vm.startSSHsession(); err != nil {
		return
	}
	defer session.close()
	return session.sCopy(source, destination, target)
}

func init() {
	RootCmd.AddCommand(sshCmd)
	RootCmd.AddCommand(scpCmd)
}
