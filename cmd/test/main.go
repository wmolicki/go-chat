package main

import (
	"fmt"
	"time"
)

func test() {
	go func() {
		t := time.NewTicker(2 * time.Second)
		for {

			select {
			case <-t.C:
				fmt.Println("tick")
			}
		}
	}()
}
func main() {
	test()
	select {}
}
