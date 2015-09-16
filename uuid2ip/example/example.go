package main

import (
	"fmt"
	"log"

	"github.com/AntonioMeireles/coreos-xhyve/uuid2ip"
)

func main() {
	var mac, ip string
	var err error
	uuid := "de742962-2a9b-4285-8595-e1c769ed7293"
	if mac, err = uuid2ip.GuestMAC(uuid); err != nil {
		log.Fatalf("%v", err)
	}
	fmt.Println(mac)
	if ip, err = uuid2ip.GuestIP(mac); err != nil {
		log.Fatalf("%v", err)
	}
	fmt.Println(ip)

}
