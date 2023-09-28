package main

import (
	"fmt"
	"os"

	"github.com/kumanik5661/exeggutor/executor"
)

func main() {
	/*
		get code from users
	*/

	/*
		compile it and generate binary executable
	*/

	var profile executor.Config
	var execstats executor.Stats
	profile.SetDefaults()

	/*
		set required profile parameters
	*/

	success, errmsg := executor.Execute(&profile, &execstats)

	if !success {
		fmt.Fprintf(os.Stderr, "%s", errmsg)
		return
	}

	fmt.Printf("success")
	fmt.Printf("mark : %s", execstats.Mark)
	fmt.Printf("CPU time : %f", execstats.CpuTime)
	fmt.Printf("elapsed time : %f", execstats.ElapsedTime)
	fmt.Printf("memory : %d", execstats.Memory)
	return
}
