// Main Process
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type config struct {
	cpu    unix.Rlimit
	aspace unix.Rlimit
	core   unix.Rlimit
	stack  unix.Rlimit
	fsize  unix.Rlimit
	nproc  unix.Rlimit

	memory uint
	clock  uint
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

const (
	NICE_LEVEL = 14
	INTERVAL   = 60
)

var defProfile = config{
	unix.Rlimit{Cur: 1, Max: 1},
	unix.Rlimit{Cur: 32768, Max: 32768},
	unix.Rlimit{Cur: 0, Max: 0},
	unix.Rlimit{Cur: 0, Max: 0},
	unix.Rlimit{Cur: 8192, Max: 8192},
	unix.Rlimit{Cur: 8192, Max: 8192},
	unix.Rlimit{Cur: 0, Max: 0},
	3,
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
var envv []string
var pageSize int = unix.Getpagesize()

func setFlags(profile *config) {
	cpu := flag.Uint64("cpu", uint64(defProfile.cpu.Cur), "CPU Limit")
	memory := flag.Uint("mem", defProfile.memory, "Memory Limit")
	aspace := flag.Uint64("space", uint64(defProfile.aspace.Cur), "Space Limit")
	minuid := flag.Int64("minuid", int64(defProfile.minuid), "Min UID")
	maxuid := flag.Int64("maxuid", int64(defProfile.maxuid), "Max UID")
	core := flag.Uint64("core", uint64(defProfile.core.Cur), "Core Limit")
	nproc := flag.Uint64("nproc", uint64(defProfile.nproc.Cur), "nproc Limit")
	fsize := flag.Uint64("fsize", uint64(defProfile.fsize.Cur), "fsize Limit")
	stack := flag.Uint64("stack", uint64(defProfile.stack.Cur), "Stack Limit")
	clock := flag.Uint("clock", uint(defProfile.clock), "Wall clock Limit in seconds")
	exec := flag.String("exec", "", "Command to execute")
	envs := flag.String("env", "", "Environment variables for execution")
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
	profile.memory = *memory
	profile.aspace.Cur, profile.aspace.Max = *aspace, *aspace
	profile.core.Cur, profile.core.Max = *core, *core
	profile.nproc.Cur, profile.nproc.Max = *nproc, *nproc
	profile.fsize.Cur, profile.fsize.Max = *fsize, *fsize
	profile.stack.Cur, profile.stack.Max = *stack, *stack
	profile.clock = *clock
	profile.minuid = int32(*minuid)
	profile.maxuid = int32(*maxuid)
	chrootDir = *fchroot
	errorFile = *ferror
	usageFile = *usage
	cmd = *exec
	envv = strings.Split((*envs), " ")
}

func exitOnError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}
}

func signalHandler() error {
	return nil
}

func setrlimits(profile config) error {
	var err error
	err = unix.Setrlimit(unix.RLIMIT_CORE, &profile.core)
	err = unix.Setrlimit(unix.RLIMIT_STACK, &profile.stack)
	err = unix.Setrlimit(unix.RLIMIT_FSIZE, &profile.fsize)
	err = unix.Setrlimit(unix.RLIMIT_NPROC, &profile.nproc)
	err = unix.Setrlimit(unix.RLIMIT_CPU, &profile.cpu)
	// Address space(including libraries) limit
	if profile.aspace.Max > 0 {
		err = unix.Setrlimit(unix.RLIMIT_AS, &profile.aspace)
	}
	return err
}

func memusage(pid int) (uint, error) {
	p, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return 0, err
	}
	pStr := string(p)
	stats := strings.Split(pStr, " ")
	nPages, err := strconv.Atoi(stats[5])
	if err != nil {
		return 0, err
	}
	mem := uint(nPages * pageSize)
	return mem, nil
}

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
		exitOnError(err)
		os.Chown(usageFile, int(profile.minuid), 0644)
		os.Chmod(usageFile, 0644)
		defer redirect.Close()
	}

	junk, err = os.OpenFile(errorFile, os.O_CREATE|os.O_RDWR, 0644)
	exitOnError(err)
	if errorFile != "/dev/null" {
		os.Chown(errorFile, int(profile.minuid), 0644)
		os.Chmod(errorFile, 0644)
	}

	err = unix.Setgid(int(profile.minuid))
	exitOnError(err)

	err = unix.Setuid(int(profile.minuid))
	exitOnError(err)

	// UID 0 is administrator user in *nix OS
	if unix.Getuid() == 0 {
		fmt.Fprintf(os.Stderr, "Not changing the uid to an unpriviledged one is a BAD idea\n")
	}

	fmt.Println("start", time.Now().UnixNano())
	pid, _, err = unix.Syscall(unix.SYS_FORK, 0, 0, 0)
	// handleErr(err)
	fmt.Println("forked")
	fmt.Println(pid)
	// Gets the process object and adds ptrace flag
	proc, err := os.FindProcess(int(pid))
	// unix.PtraceAttach(int(pid))

	unix.Alarm(uint(profile.clock))

	if pid == 0 {
		// Forked/Child Process
		proc, err := os.FindProcess(unix.Getpid())
		exitOnError(err)
		// Chrooting
		if chrootDir != "/tmp" {
			err = unix.Chdir(chrootDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Cannot channge to chroot dir")
				proc.Signal(unix.SIGPIPE)
			}
			err = unix.Chroot(chrootDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Cannot channge to chroot dir")
				proc.Signal(unix.SIGPIPE)
			}
		}
		// Open junk file instead of stderr
		err = unix.Dup2(int(junk.Fd()), int(os.Stderr.Fd()))
		exitOnError(err)
		// Set UID for the process
		unix.Setuid(int(profile.minuid))
		exitOnError(err)
		// Check if running as root
		if unix.Getuid() == 0 {
			fmt.Fprintf(os.Stderr, "Running as a root is not secure!")
			os.Exit(1)
		}
		// Set Priority
		err = unix.Setpriority(unix.PRIO_USER, int(profile.minuid), NICE_LEVEL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Couldn't set priority")
			proc.Signal(unix.SIGPIPE)
		}
		// Setrlimit syscalls
		err = setrlimits(profile)

		cmdArr := strings.Split(cmd, " ")
		// Start execution of user program
		err = unix.Exec(cmdArr[0], cmdArr, envv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Couldn't run the program")
			proc.Signal(unix.SIGPIPE)
		}
	} else {
		// Parent
		var ws unix.WaitStatus
		var rusage unix.Rusage
		skips := 0
		ticker := time.NewTicker(INTERVAL * time.Millisecond)
		for {
			<-ticker.C
			wpid, err := unix.Wait4(proc.Pid, &ws, unix.WNOHANG|unix.WUNTRACED, &rusage)
			exitOnError(err)
			if wpid == -1 {
				fmt.Fprintf(os.Stderr, "Process has exited cannot trace further")
				os.Exit(1)
			}
			if wpid == proc.Pid && ws.Exited() {
				ticker.Stop()
				break
			}
			if !ws.Exited() {
				// err := unix.PtraceSingleStep(wpid)
				mem, err := memusage(proc.Pid)
				if err != nil {
					skips++
				}
				if skips > 10 {
					mark = MLE
					err := proc.Kill()
					exitOnError(err)
				} else if mem > profile.memory {
					mark = MLE
					err := proc.Kill()
					exitOnError(err)
				}
			}
		}
	}
	fmt.Println("EXITING", os.Getpid())
}
