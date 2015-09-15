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

// Package uuid2ip is a simple interface to interact with xhyve's networking
package uuid2ip

/*
#cgo CFLAGS: -framework Hypervisor -framework vmnet
#cgo LDFLAGS: -framework Hypervisor -framework vmnet
#include <stdlib.h>
#include "uuid2ip.c"
*/
import "C"
import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"runtime"
	"strings"
	"unsafe"
)

// GuestMACfromUUID returns the MAC address that will assembled from the given
// UUID by xhyve,  needs to be called before xhyve actual invocation
func GuestMACfromUUID(uuid string) (mac string, err error) {
	fail, uuidC := "", C.CString(uuid)
	macC, failC := C.CString(mac), C.CString(fail)
	var ret C.int

	defer func() {
		C.free(unsafe.Pointer(uuidC))
		C.free(unsafe.Pointer(macC))
		C.free(unsafe.Pointer(failC))
	}()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if ret = C.vmnet_get_mac_address_from_uuid(uuidC,
		(*C.char)(unsafe.Pointer(macC)),
		(*C.char)(unsafe.Pointer(failC))); ret != 0 {
		return mac, fmt.Errorf(C.GoString(failC))
	}
	return C.GoString(macC), nil
}

// GuestIPfromMAC returns the IP address that would be leased to the given MAC
// address by xhyve, to be called after actual xhyve invocation
func GuestIPfromMAC(mac string) (ip string, err error) {
	var (
		f          *os.File
		allLeases  []byte
		lastFound  string
		leasesPath = "/var/db/dhcpd_leases"
		ipAddrRe   = regexp.MustCompile(`^.*ip_address=(.+?)$`)
		macAddrRe  = regexp.MustCompile(`^.*hw_address=1,(.+?)$`)
	)

	if f, err = os.Open(leasesPath); err != nil {
		return
	}
	defer f.Close()

	if allLeases, err = ioutil.ReadAll(f); err != nil {
		return
	}

	for _, l := range strings.Split(string(allLeases), "\n") {
		l = strings.TrimRight(l, "\r")

		matches := ipAddrRe.FindStringSubmatch(l)
		if matches != nil {
			lastFound = matches[1]
			continue
		}

		matches = macAddrRe.FindStringSubmatch(l)
		// OSX's dhcp stores technically incorrect mac addr as it strips
		// leading zeros from them, so we need to take that in consideration
		// before checking match...
		if matches != nil && strings.EqualFold(matches[1],
			strings.TrimPrefix(strings.Replace(mac, ":0", ":", -1), "0")) {
			return lastFound, nil
		}
	}
	return ip, fmt.Errorf("%s isn't in ANY active DHCP lease (!)", mac)
}
