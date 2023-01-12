package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func test() {
	p, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", 1))
	if err != nil {
		log.Fatal(err)
	}
	pStr := string(p)
	stats := strings.Split(pStr, " ")
	pageSize := unix.Getpagesize()
	nPages, err := strconv.Atoi(stats[5])
	if err != nil {
		log.Fatal(err)
	}
	mem := nPages * pageSize
	fmt.Println("Page Size:", pageSize)
	fmt.Println("No. of Pages: ", nPages)
	fmt.Println("Memory Usage", mem)
}
