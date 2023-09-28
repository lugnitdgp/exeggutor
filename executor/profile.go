package executor

import "golang.org/x/sys/unix"

type Config struct {
	Cpu       unix.Rlimit
	Aspace    unix.Rlimit
	Core      unix.Rlimit
	Stack     unix.Rlimit
	Fsize     unix.Rlimit
	Nproc     unix.Rlimit
	Memory    unix.Rlimit
	Clock     uint
	Minuid    int32
	Maxuid    int32
	ChrootDir string
	// ErrorFile string
	// UsageFile string
	InFile  string
	OutFile string
	Cmd     string
	Envv    []string
}

func (conf *Config) SetDefaults() {
	conf.Cpu.Cur = 1024
	conf.Cpu.Max = 1
	conf.Aspace.Cur = 1024
	conf.Aspace.Max = 0
	conf.Core.Cur = 1024
	conf.Core.Max = 0
	conf.Stack.Cur = 8192
	conf.Stack.Max = 8192
	conf.Fsize.Cur = 8192
	conf.Fsize.Max = 8192
	conf.Nproc.Cur = 1
	conf.Nproc.Max = 0
	conf.Memory.Cur = 32768
	conf.Memory.Max = 32768
	conf.Clock = 1
	conf.Minuid = 5000
	conf.Maxuid = 65535
	conf.ChrootDir = "/tmp"
	// conf.ErrorFile = "/dev/null"
	// conf.UsageFile = "/dev/null"
	conf.OutFile = "./output/out.txt"
	conf.InFile = "/dev/null"
}
