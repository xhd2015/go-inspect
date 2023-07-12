package sh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/xhd2015/go-inspect/sh/process"
)

func RunBash(cmdList []string, verbose bool) error {
	_, _, err := RunBashWithOpts(cmdList, RunBashOptions{
		Verbose: verbose,
	})
	return err
}

type RunBashOptions struct {
	Verbose    bool
	NeedStdErr bool
	NeedStdOut bool

	Args []string

	ErrExcludeCmd bool

	// if StdoutToJSON != nil, the value is parsed into this struct
	StdoutToJSON interface{}
	FilterCmd    func(cmd *exec.Cmd)
}

func RunBashWithOpts(cmdList []string, opts RunBashOptions) (stdout string, stderr string, err error) {
	cmdExpr := bashCommandExpr(cmdList)
	if opts.Verbose {
		log.Printf("%s", cmdExpr)
	}
	list := make([]string, 2+len(opts.Args))
	list[0] = "-c"
	list[1] = cmdExpr
	for i, arg := range opts.Args {
		list[i+2] = arg
	}

	// bash -c cmdExpr args...
	cmd := exec.Command("bash", list...)
	stdoutBuf := bytes.NewBuffer(nil)
	stderrBuf := bytes.NewBuffer(nil)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	if opts.FilterCmd != nil {
		opts.FilterCmd(cmd)
	}
	process.SetSysProcAttribute(cmd)
	err = cmd.Run()
	if err != nil {
		cmdDetail := ""
		if !opts.ErrExcludeCmd {
			cmdDetail = fmt.Sprintf("cmd %s ", cmdExpr)
		}
		err = fmt.Errorf("running cmd error: %s%v stdout:%s stderr:%s", cmdDetail, err, stdoutBuf.String(), stderrBuf.String())
		return
	}
	if opts.NeedStdOut {
		stdout = stdoutBuf.String()
	}
	if opts.NeedStdErr {
		stderr = stderrBuf.String()
	}
	if opts.StdoutToJSON != nil {
		err = json.Unmarshal(stdoutBuf.Bytes(), opts.StdoutToJSON)
		if err != nil {
			err = fmt.Errorf("parse command output to %T error:%v", opts.StdoutToJSON, err)
		}
	}
	return
}

func JoinArgs(args []string) string {
	eArgs := make([]string, 0, len(args))
	for _, arg := range args {
		eArgs = append(eArgs, Quote(arg))
	}
	return strings.Join(eArgs, " ")
}

func Quotes(args ...string) string {
	eArgs := make([]string, 0, len(args))
	for _, arg := range args {
		eArgs = append(eArgs, Quote(arg))
	}
	return strings.Join(eArgs, " ")
}
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.ContainsAny(s, "\t \n;<>\\${}()&!*") { // special args
		s = strings.ReplaceAll(s, "'", "'\\''")
		return "'" + s + "'"
	}
	return s
}

func bashCommandExpr(cmd []string) string {
	var b strings.Builder
	for i, c := range cmd {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		b.WriteString(c)
		if i >= len(cmd)-1 {
			// last no \n
			continue
		}
		if strings.HasSuffix(c, "\n") || strings.HasSuffix(c, "&&") || strings.HasSuffix(c, ";") || strings.HasSuffix(c, "||") {
			continue
		}
		b.WriteString("\n")
	}
	return strings.Join(cmd, "\n")
}
