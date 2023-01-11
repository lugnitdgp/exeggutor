package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"syscall"
	"time"
)

type config struct {
	cpu    syscall.Rlimit
	memory syscall.Rlimit
	aspace syscall.Rlimit
	core   syscall.Rlimit
	stack  syscall.Rlimit
	fsize  syscall.Rlimit
	nproc  syscall.Rlimit
	clock  syscall.Rlimit

	minuid int32
	maxuid int32
}

const (
	OK   = iota /* OK process finished normally */
	OLE         /* OLE output limit exceeded */
	MLE         /* MLE memory limit exceeded */
	TLE         /* TLE time limit exceeded */
	RTLE        /* RTLE time limit exceeded(wall clock) */
	RF          /* RF invalid function */
	IE          /* IE internal error */
)

var defProfile = config{
	syscall.Rlimit{Cur: 1, Max: 1},
	syscall.Rlimit{Cur: 32768, Max: 32768},
	syscall.Rlimit{Cur: 0, Max: 0},
	syscall.Rlimit{Cur: 0, Max: 0},
	syscall.Rlimit{Cur: 8192, Max: 8192},
	syscall.Rlimit{Cur: 8192, Max: 8192},
	syscall.Rlimit{Cur: 0, Max: 0},
	syscall.Rlimit{Cur: 3, Max: 3},
	5000,
	65535,
}

var chrootDir = "/tmp"
var errorFile = "/dev/null"
var usageFile = "/dev/null"
var cmd = ""
var redirect *os.File
var junk *os.File
var mark int
var pid uintptr

func setFlags(profile *config) {
	cpu := flag.Uint64("cpu", uint64(defProfile.cpu.Cur), "CPU Limit")
	memory := flag.Uint64("mem", uint64(defProfile.memory.Cur), "Memory Limit")
	aspace := flag.Uint64("space", uint64(defProfile.memory.Cur), "Space Limit")
	minuid := flag.Int64("minuid", int64(defProfile.minuid), "Min UID")
	maxuid := flag.Int64("maxuid", int64(defProfile.maxuid), "Max UID")
	core := flag.Uint64("core", uint64(defProfile.core.Cur), "Core Limit")
	nproc := flag.Uint64("nproc", uint64(defProfile.nproc.Cur), "nproc Limit")
	fsize := flag.Uint64("fsize", uint64(defProfile.fsize.Cur), "fsize Limit")
	stack := flag.Uint64("stack", uint64(defProfile.stack.Cur), "Stack Limit")
	clock := flag.Uint64("clock", uint64(defProfile.clock.Cur), "Wall clock Limit in seconds")
	exec := flag.String("exec", "", "Command to execute")
	fchroot := flag.String("chroot", "/tmp", "Directory to chroot")
	ferror := flag.String("error", "/dev/null", "Print stderr to")
	usage := flag.String("usage", "/dev/null", "Report Statistics to")

	flag.Parse()

	if *exec == "" {
		fmt.Fprintf(os.Stderr, "missing required exec argument\n")
		flag.PrintDefaults()
		os.Exit(2)
	}

	profile.cpu.Cur, profile.cpu.Max = *cpu, *cpu
	profile.memory.Cur, profile.memory.Max = *memory, *memory
	profile.aspace.Cur, profile.aspace.Max = *aspace, *aspace
	profile.core.Cur, profile.core.Max = *core, *core
	profile.nproc.Cur, profile.nproc.Max = *nproc, *nproc
	profile.fsize.Cur, profile.fsize.Max = *fsize, *fsize
	profile.stack.Cur, profile.stack.Max = *stack, *stack
	profile.clock.Cur, profile.clock.Max = *clock, *clock
	profile.minuid = int32(*minuid)
	profile.maxuid = int32(*maxuid)
	chrootDir = *fchroot
	errorFile = *ferror
	usageFile = *usage
	cmd = *exec
}

func handleErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}
}

func alarm(seconds int, callback func()) (endRecSignal chan string) {
	endRecSignal = make(chan string)
	go func() {
		time.AfterFunc(time.Duration(seconds)*time.Second, func() {
			callback()
			endRecSignal <- "time up"
			close(endRecSignal)
		})
	}()
	return
}

// func handleSignals(signal os.Signal) {
//     switch signal {
//     case syscall.SIGALRM:

//     }
// }

// TODO: Change file permissions to 0640 in production

func main() {
	profile := defProfile
	setFlags(&profile)

	// var tstart, tfinish time.Time
	redirect = os.Stderr
	var err error

	// Get an unused UID
	if profile.minuid != profile.maxuid {
		seed := rand.NewSource(time.Now().UnixNano())
		rand1 := rand.New(seed)
		profile.minuid += rand1.Int31n(profile.maxuid - profile.minuid)
	}

	// Opening usage and error files for o/p of this program and error o/p of user program
	if usageFile != "/dev/null" {
		redirect, err = os.OpenFile(usageFile, os.O_CREATE|os.O_RDWR, 0644)
		handleErr(err)
		os.Chown(usageFile, int(profile.minuid), 0644)
		os.Chmod(usageFile, 0644)
		defer redirect.Close()
	}

	junk, err = os.OpenFile(errorFile, os.O_CREATE|os.O_RDWR, 0644)
	handleErr(err)
	if errorFile != "/dev/null" {
		os.Chown(errorFile, int(profile.minuid), 0644)
		os.Chmod(errorFile, 0644)
	}

	err = syscall.Setgid(int(profile.minuid))
	handleErr(err)

	err = syscall.Setuid(int(profile.minuid))
	handleErr(err)

	// UID 0 is administrator user in *nix OS
	if syscall.Getuid() == 0 {
		fmt.Fprintf(os.Stderr, "Not changing the uid to an unpriviledged one is a BAD idea\n")
	}

	fmt.Println("start", time.Now().UnixNano())
	pid, _, err = syscall.Syscall(syscall.SYS_FORK, 0, 0, 0)
	// handleErr(err)
	fmt.Println("forked")
	fmt.Println(pid)

	if pid == 0 {
		// Forked/Child Process
		for {
		}
	} else {
		// Parent
		proc, err := os.FindProcess(int(pid))

		alarm(int(profile.clock.Max), func() {
			mark = RTLE
			fmt.Println("\n", pid, "alarm")
			if pid != 0 {
				handleErr(err)
				// proc.Signal(syscall.SIGALRM)
				fmt.Println("end", time.Now().UnixNano())
				proc.Kill()
				fmt.Println("Killed", pid)
			}
		})

		_, err = proc.Wait()
		handleErr(err)
	}
	fmt.Println("EXITING", os.Getpid())
}
