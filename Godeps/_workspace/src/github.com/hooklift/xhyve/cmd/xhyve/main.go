package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/hooklift/xhyve"
)

func init() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	go func() {
		s := <-c
		fmt.Printf("Signal %s received\n", s)
	}()
}

func main() {
	done := make(chan bool)
	go func() {
		if err := xhyve.Run(os.Args); err != nil {
			fmt.Println(err)
		}
		done <- true
	}()

	<-done
	fmt.Println("Hypervisor goroutine finished!")
}
