// Main Process
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
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

var defProfile = config{
	unix.Rlimit{Cur: 1024, Max: 1},
	unix.Rlimit{Cur: 1024, Max: 0},
	unix.Rlimit{Cur: 1024, Max: 0},
	unix.Rlimit{Cur: 8192, Max: 8192},
	unix.Rlimit{Cur: 8192, Max: 8192},
	unix.Rlimit{Cur: 1, Max: 0},
	32768,
	1,
	5000,
	65535,
}

const (
	OK   = "OK\n"   /* OK : process finished normally */
	OLE  = "OLE\n"  /* OLE : output limit exceeded */
	MLE  = "MLE\n"  /* MLE : memory limit exceeded */
	TLE  = "TLE\n"  /* TLE : time limit exceeded */
	RTLE = "RTLE\n" /* RTLE : time limit exceeded(wall clock) */
	RF   = "RF\n"   /* RF : invalid function */
	IE   = "IE\n"   /* IE : internal error */
	NZEC = "NZEC\n" /* NZEC : Non-Zero Error Code */
)

const (
	niceLvl  = 14
	interval = 1
)

var chrootDir = "/tmp"
var errorFile = "/dev/null"
var usageFile = "/dev/null"
var outFile = "./out.txt"
var cmd = ""
var usageFp *os.File
var junkFp *os.File
var outFp *os.File
var mark string
var pid uintptr
var envv []string
var pageSize int = unix.Getpagesize()
var mem uint
var ns2s = 1000000000.000

func boolSolver(b bool) string {
	if b == true {
		return "true"
	}
	return "false"
}

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
	outfile := flag.String("outfile", "./out.txt", "Print output to file")

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
	outFile = *outfile
	cmd = *exec
	envv = strings.Split((*envs), " ")
}

func exitOnError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}
}

// * Error in test case: stack size, fsize, nproc
func setrlimits(profile config) error {
	var err error
	err = unix.Setrlimit(unix.RLIMIT_CORE, &profile.core)
	if err != nil {
		return err
	}
	err = unix.Setrlimit(unix.RLIMIT_STACK, &profile.stack)
	if err != nil {
		return err
	}
	err = unix.Setrlimit(unix.RLIMIT_FSIZE, &profile.fsize)
	if err != nil {
		return err
	}
	err = unix.Setrlimit(unix.RLIMIT_NPROC, &profile.nproc)
	if err != nil {
		return err
	}
	err = unix.Setrlimit(unix.RLIMIT_CPU, &profile.cpu)
	if err != nil {
		return err
	}
	// Address space(including libraries) limit
	if profile.aspace.Max > 0 {
		err = unix.Setrlimit(unix.RLIMIT_AS, &profile.aspace)
	}
	return err
}

// func printstats(byts string) {
// 	redirect.Seek(0, 2)
// 	redirect.WriteString(byts)
// }

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

func printstats(byts string) {
	usageFp.Seek(0, 2)
	usageFp.WriteString(byts)
}

// TODO: Change file permissions to 0640 in production

func main() {
	profile := defProfile
	setFlags(&profile)

	var tstart, tfinish int64
	usageFp = os.Stderr
	var err error

	// Get an unused UID
	if profile.minuid != profile.maxuid {
		seed := rand.NewSource(time.Now().UnixNano())
		rand1 := rand.New(seed)
		profile.minuid += rand1.Int31n(profile.maxuid - profile.minuid)
	}

	/*	Opening usage and error files for
		o/p of this program and error o/p of user program */
	if usageFile != "/dev/null" {
		usageFp, err = os.OpenFile(usageFile, os.O_CREATE|os.O_RDWR, 0644)
		exitOnError(err)
		os.Truncate(usageFile, 0)
		os.Chown(usageFile, int(profile.minuid), 0644)
		os.Chmod(usageFile, 0644)
		defer usageFp.Close()
	}

	junkFp, err = os.OpenFile(errorFile, os.O_CREATE|os.O_RDWR, 0644)
	exitOnError(err)
	junkFp.Truncate(0)
	if errorFile != "/dev/null" {
		os.Chown(errorFile, int(profile.minuid), 0644)
		os.Chmod(errorFile, 0644)
	}
	defer junkFp.Close()

	outp, err := os.OpenFile(outFile, os.O_CREATE|os.O_RDWR, 0644)
	exitOnError(err)
	outp.Truncate(0)
	os.Chown(outFile, int(profile.minuid), 0644)
	os.Chmod(outFile, 0644)
	defer outp.Close()

	err = unix.Setgid(int(profile.minuid))
	exitOnError(err)

	err = unix.Setuid(int(profile.minuid))
	exitOnError(err)

	// UID 0 is administrator user in *nix OS
	if unix.Getuid() == 0 {
		fmt.Fprintf(os.Stderr, "Not changing the uid to an unpriviledged one is a BAD idea\n")
	}

	/*	Sets Wall Clock timer for parent process
		parent intercepts the signal and kills the child and sets TLE */
	unix.Alarm(uint(profile.clock))

	// Signal Handler for SIGALRM
	c := make(chan os.Signal, 1)
	signal.Notify(c, unix.SIGALRM)
	go func() {
		<-c
		fmt.Println("Received SIGALRM")
		mark = TLE
		fmt.Print(mark)
		proc, err := os.FindProcess((int(pid)))
		if err == nil {
			proc.Kill()
		} else {
			fmt.Println("TLE but cannot kill child process")
		}
		tfinish = time.Now().UnixNano()
	}()

	fmt.Println("start", time.Now().UnixNano())
	tstart = time.Now().UnixNano()

	pid, _, err = unix.Syscall(unix.SYS_FORK, 0, 0, 0)
	// Gets the process object (and adds ptrace flag)
	proc, err := os.FindProcess(int(pid))
	// unix.PtraceAttach(int(pid))

	if pid == 0 {
		// Forked/Child Process
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

		// fmt.Println("UIDs: ", profile.minuid, "  ", unix.Getuid())

		// Check if running as root
		if unix.Getuid() == 0 {
			fmt.Fprintf(os.Stderr, "Running as a root is not secure!")
			os.Exit(1)
		}

		// Set Priority
		err = unix.Setpriority(unix.PRIO_USER, int(profile.minuid), niceLvl)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Couldn't set priority")
			proc.Signal(unix.SIGPIPE)
		}

		// Setrlimit syscalls
		setrlimits(profile)
		fmt.Println("SETRLIMITS ERROR:", err)

		// TODO: Keep 267,8 commented in dev
		// // Open junk file instead of stderr
		// err = unix.Dup2(int(junk.Fd()), int(os.Stderr.Fd()))
		// exitOnError(err)

		// Use out file instead of stdout
		err = unix.Dup2(int(outp.Fd()), int(os.Stdout.Fd()))
		exitOnError(err)

		cmdArr := []string{"/bin/bash", "-c", cmd}
		// fmt.Println(cmdArr)
		// Start execution of user program
		err = unix.Exec("/bin/bash", cmdArr, envv)
		if err != nil {
			fmt.Printf("Couldn't run the program")
			proc.Signal(unix.SIGPIPE)
		}
	} else {
		state, err := proc.Wait()
		exitOnError(err)
		fmt.Printf("EXITED: %s\tEXIT CODE: %d\n", boolSolver(state.Exited()), state.ExitCode())

		// ticker := time.NewTicker(INTERVAL * time.Millisecond)
		// var ws unix.WaitStatus
		// var rusage unix.Rusage
		// for {
		// 	<-ticker.C
		// 	fmt.Println("Polling process activity")
		// 	fmt.Println(time.Now().UnixNano())
		// 	_, err := unix.Wait4(proc.Pid, &ws, unix.WNOHANG|unix.WUNTRACED|unix.WCONTINUED, &rusage)
		// 	if err != nil {
		// 		panic(err)
		// 	}
		// 	fmt.Println(time.Now().UnixNano())
		// 	if ws.Exited() {
		// 		fmt.Printf("PID: %d\nEXIT_CODE: %d\n", proc.Pid, ws.ExitStatus())
		// 		ticker.Stop()
		// 		break
		// 	}
		// 	if !ws.Exited() {
		// 		// fmt.Println("SYS_USAGE:", (state.SysUsage()))
		// 		mem, _ := memusage(proc.Pid)
		// 		fmt.Printf("MEM_USAGE: %d\n", mem)
		// 	}
		// }

	}
	tfinish = time.Now().UnixNano()
	fmt.Printf("TIME: %.03f s\n", float64(tfinish-tstart)/ns2s)
	fmt.Println("EXITING", os.Getpid())
}
