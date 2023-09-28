package executor

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var signalNames = []string{
	"UNKONWN",   /*  0 */
	"SIGHUP",    /*  1 */
	"SIGINT",    /*  2 */
	"SIGQUIT",   /*  3 */
	"SIGILL",    /*  4 */
	"SIGTRAP",   /*  5 */
	"SIGABRT",   /*  6 */
	"SIGBUS",    /*  7 */
	"SIGFPE",    /*  8 */
	"SIGKILL",   /*  9 */
	"SIGUSR1",   /* 10 */
	"SIGSEGV",   /* 11 */
	"SIGUSR2",   /* 12 */
	"SIGPIPE",   /* 13 */
	"SIGALRM",   /* 14 */
	"SIGTERM",   /* 15 */
	"SIGSTKFLT", /* 16 */
	"SIGCHLD",   /* 17 */
	"SIGCONT",   /* 18 */
	"SIGSTOP",   /* 19 */
	"SIGTSTP",   /* 20 */
	"SIGTTIN",   /* 21 */
	"SIGTTOU",   /* 22 */
	"SIGURG",    /* 23 */
	"SIGXCPU",   /* 24 */
	"SIGXFSZ",   /* 25 */
	"SIGVTALRM", /* 26 */
	"SIGPROF",   /* 27 */
	"SIGWINCH",  /* 28 */
	"SIGIO",     /* 29 */
	"SIGPWR",    /* 30 */
	"SIGSYS",    /* 31 */
}

const (
	OK   = "OK\n"      /* OK : process finished normally */
	OLE  = "OLE\n"     /* OLE : output limit exceeded */
	MLE  = "MLE\n"     /* MLE : memory limit exceeded */
	TLE  = "TLE\n"     /* TLE : time limit exceeded */
	RTLE = "RTLE\n"    /* RTLE : time limit exceeded(wall clock) */
	RF   = "RF\n"      /* RF : invalid function */
	IE   = "IE\n"      /* IE : internal error */
	NZEC = "NZEC %d\n" /* NZEC : Non-Zero Error Code */
)

const (
	niceLvl  = 14
	interval = 15
)

// var usageFp *os.File
// var junkFp *os.File
var mark string
var pid uintptr
var pageSize int = unix.Getpagesize() / 1024

// var mem uint
var ns2s = 1000000000.000

func boolSolver(b bool) string {
	if b == true {
		return "true"
	}
	return "false"
}

/*func setFlags(profile *config) {
	cpu := flag.Uint64("cpu", uint64(defProfile.cpu.Cur), "CPU Limit")
	memory := flag.Uint64("mem", uint64(defProfile.memory.Cur), "Memory Limit")
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
	profile.Memory.Cur, profile.Memory.Max = *memory, *memory
	profile.aspace.Cur, profile.aspace.Max = *aspace, *aspace
	profile.core.Cur, profile.core.Max = *core, *core
	profile.nproc.Cur, profile.nproc.Max = *nproc, *nproc
	profile.fsize.Cur, profile.fsize.Max = *fsize, *fsize
	profile.stack.Cur, profile.stack.Max = *stack, *stack
	profile.clock = *clock
	profile.Minuid = int32(*minuid)
	profile.Maxuid = int32(*maxuid)
	profile.ChrootDir = *fchroot
	profile.ErrorFile = *ferror
	profile.UsageFile = *usage
	profile.OutFile = *outfile
	profile.Cmd = *exec
	profile.Envv = strings.Split((*envs), " ")
}*/

func exitOnError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}
}

// * Error in test case: stack size, fsize, nproc
func setrlimits(profile *Config) error {
	var err error
	err = unix.Setrlimit(unix.RLIMIT_CORE, &profile.Core)
	if err != nil {
		return err
	}
	err = unix.Setrlimit(unix.RLIMIT_STACK, &profile.Stack)
	if err != nil {
		return err
	}
	err = unix.Setrlimit(unix.RLIMIT_FSIZE, &profile.Fsize)
	if err != nil {
		return err
	}
	err = unix.Setrlimit(unix.RLIMIT_NPROC, &profile.Nproc)
	if err != nil {
		return err
	}
	err = unix.Setrlimit(unix.RLIMIT_CPU, &profile.Cpu)
	if err != nil {
		return err
	}
	err = unix.Setrlimit(unix.RLIMIT_MEMLOCK, &profile.Memory)
	if err != nil {
		return err
	}
	// Address space(including libraries) limit
	if profile.Aspace.Cur > 0 {
		err = unix.Setrlimit(unix.RLIMIT_AS, &profile.Aspace)
	}
	return err
}

func memusage(pid int) (int, error) {
	statm, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return 0, err
	}
	statmStr := string(statm)
	statmArr := strings.Split(statmStr, " ")
	nPages, err := strconv.Atoi(statmArr[5])
	if err != nil {
		return 0, err
	}
	mem := nPages * pageSize
	return mem, nil
}

func timeusage(pid int) (int, int, error) {
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, 0, err
	}
	statStr := string(stat)
	statArr := strings.Split(statStr, " ")
	utime, err := strconv.Atoi(statArr[13])
	stime, err := strconv.Atoi(statArr[14])
	return utime, stime, err
}

/*func printstats(byts string) {
	usageFp.Seek(0, 2)
	usageFp.WriteString(byts)
}
*/
// TODO: Change file permissions to 0640 in production

func Execute(profile *Config, stats *Stats) (bool, string) {
	// profile := defProfile
	// setFlags(&profile)

	var tstart, tfinish int64
	var mem, skips int
	var utime int64
	var err error

	// Get an unused UID
	if profile.Minuid != profile.Maxuid {
		seed := rand.NewSource(time.Now().UnixNano())
		rand1 := rand.New(seed)
		profile.Minuid += rand1.Int31n(profile.Maxuid - profile.Minuid)
	}

	/*	Opening usage and error files for
		o/p of this program and error o/p of user program */

	if profile.InFile == "/dev/null" {
		return false, "missing input file"
	}

	inp, err := os.OpenFile(profile.InFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return false, err.Error()
	}

	inp.Truncate(0)
	os.Chown(profile.InFile, int(profile.Minuid), 0444)
	os.Chmod(profile.InFile, 0444)
	defer inp.Close()

	// if profile.UsageFile != "/dev/null" {
	// 	usageFp, err = os.OpenFile(profile.UsageFile, os.O_CREATE|os.O_RDWR, 0644)
	// 	exitOnError(err)
	// 	os.Truncate(profile.UsageFile, 0)
	// 	os.Chown(profile.UsageFile, int(profile.Minuid), 0644)
	// 	os.Chmod(profile.UsageFile, 0644)
	// 	defer usageFp.Close()
	// }

	// if profile.ErrorFile != "/dev/null" {
	// 	junkFp, err = os.OpenFile(profile.ErrorFile, os.O_CREATE|os.O_RDWR, 0644)
	// 	exitOnError(err)
	// 	junkFp.Truncate(0)
	// 	os.Chown(profile.ErrorFile, int(profile.Minuid), 0644)
	// 	os.Chmod(profile.ErrorFile, 0644)
	// 	defer junkFp.Close()
	// }

	outp, err := os.OpenFile(profile.OutFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return false, err.Error()
	}

	outp.Truncate(0)
	os.Chown(profile.OutFile, int(profile.Minuid), 0644)
	os.Chmod(profile.OutFile, 0644)
	defer outp.Close()

	err = unix.Setgid(int(profile.Minuid))
	if err != nil {
		return false, err.Error()
	}

	err = unix.Setuid(int(profile.Minuid))
	if err != nil {
		return false, err.Error()
	}

	// UID 0 is administrator user in *nix OS
	if unix.Getuid() == 0 {
		fmt.Fprintf(os.Stderr, "Not changing the uid to an unpriviledged one is a BAD idea\n")
	}

	/*	Sets Wall Clock timer for parent process
		parent intercepts the signal and kills the child and sets RTLE */
	unix.Alarm(uint(profile.Clock))

	// Signal Handler for SIGALRM
	c := make(chan os.Signal, 1)
	signal.Notify(c, unix.SIGALRM)
	go func() {
		<-c
		mark = RTLE
		proc, err := os.FindProcess((int(pid)))
		if err == nil {
			proc.Kill()
		} else {
			fmt.Println("TLE but cannot kill child process")
		}
		tfinish = time.Now().UnixNano()
	}()

	tstart = time.Now().UnixNano()

	pid, _, err = unix.Syscall(unix.SYS_FORK, 0, 0, 0)
	// Gets the process object (and adds ptrace flag)
	proc, err := os.FindProcess(int(pid))
	// unix.PtraceAttach(int(pid))

	if pid == 0 {
		// Forked/Child Process
		// Chrooting
		if profile.ChrootDir != "/tmp" {
			err = unix.Chdir(profile.ChrootDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Cannot channge to chroot dir")
				proc.Signal(unix.SIGPIPE)
			}
			err = unix.Chroot(profile.ChrootDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Cannot channge to chroot dir")
				proc.Signal(unix.SIGPIPE)
			}
		}

		// fmt.Println("UIDs: ", profile.Minuid, "  ", unix.Getuid())

		// Check if running as root
		if unix.Getuid() == 0 {
			fmt.Fprintf(os.Stderr, "Running as a root is not secure!")
			os.Exit(1)
		}

		// Set Priority
		err = unix.Setpriority(unix.PRIO_USER, int(profile.Minuid), niceLvl)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Couldn't set priority")
			proc.Signal(unix.SIGPIPE)
		}

		// Setrlimit syscalls
		setrlimits(profile)
		// fmt.Println("SETRLIMITS ERROR:", err)

		// TODO: Keep 267,8 commented in dev
		// // Open junk file instead of stderr
		// err = unix.Dup2(int(junk.Fd()), int(os.Stderr.Fd()))
		// exitOnError(err)

		// Use out file instead of stdout
		err = unix.Dup2(int(outp.Fd()), int(os.Stdout.Fd()))
		exitOnError(err)

		// Use in file instead of stdout
		err = unix.Dup2(int(inp.Fd()), int(os.Stdin.Fd()))
		exitOnError(err)

		cmdArr := []string{"/bin/bash", "-c", profile.Cmd}
		// fmt.Println(profile.CmdArr)
		// Start execution of user program
		err = unix.Exec("/bin/bash", cmdArr, profile.Envv)
		if err != nil {
			fmt.Printf("Couldn't run the program")
			proc.Signal(unix.SIGPIPE)
		}
	} else {
		var ws unix.WaitStatus
		var rusage unix.Rusage
		ticker := time.NewTicker(interval * time.Millisecond)

		for {
			<-ticker.C
			// fmt.Println("\nPolling process activity")
			ws = *new(unix.WaitStatus)
			rusage = *new(unix.Rusage)
			wpid, err := unix.Wait4(proc.Pid, &ws, unix.WNOHANG|unix.WUNTRACED, &rusage)
			if err != nil {
				fmt.Println(err)
			}
			if wpid == 0 {
				// Child Process hasn't died and we can check its resource utilization
				mem, err = memusage(proc.Pid)
				utime = rusage.Utime.Sec
				// fmt.Printf("UTIME: %d %d\n", rusage.Utime.Sec, rusage.Utime.Usec)
				// fmt.Printf("STIME: %d %d\n", rusage.Stime.Sec, rusage.Stime.Usec)
				if err != nil {
					skips++
				}
				if skips > 10 || mem > int(profile.Memory.Cur) {
					proc.Kill()
					mark = MLE
				}
			} else {
				// Child Process has died
				if ws.Exited() && ws.ExitStatus() != 0 {
					mark = fmt.Sprintf(NZEC, ws.ExitStatus())
				} else if ws.Stopped() && ws.StopSignal() != 0 {
					mark = fmt.Sprintf("Command terminated by signal (%d: %s)\n", ws.StopSignal(), ws.StopSignal().String())
				} else if ws.Signaled() {
					if ws.Signal() == syscall.SIGKILL {
						mark = TLE
					} else if ws.Signal() == syscall.SIGXFSZ {
						mark = OLE
					} else if ws.Signal() == syscall.SIGHUP {
						mark = RF
					} else if ws.Signal() == syscall.SIGPIPE {
						mark = IE
					} else {
						mark = fmt.Sprintf("Program terminated by signal (%d: %s)\n", ws.Signal(), ws.Signal().String())
					}
				} else {
					mark = OK
				}
				// Stop the ticker and exit polling loop
				ticker.Stop()
				break
			}
		}
	}

	tfinish = time.Now().UnixNano()
	// printstats(mark)
	// printstats(fmt.Sprintf("ELAPSED_TIME: %.03f s\n", float64(tfinish-tstart)/ns2s))
	// printstats(fmt.Sprintf("MEMORY_USED: %d kB\n", mem))
	// printstats(fmt.Sprintf("CPU_TIME: %.03f s\n", float64(utime)))

	stats.CpuTime = float64(utime)
	stats.ElapsedTime = float64(tfinish-tstart) / ns2s
	stats.Memory = mem
	stats.Mark = mark

	fmt.Printf("TIME: %.03f s\n", stats.ElapsedTime)
	fmt.Println("EXITING", os.Getpid())

	return true, ""
}
