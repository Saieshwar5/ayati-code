package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	"github.com/Saieshwar5/ayati-code/internal/session"
)

const maxInputBytes = 1 << 20

var ErrInputTooLarge = errors.New("input exceeds 1 MiB")

type Console struct {
	reader *bufio.Reader
	out    io.Writer
	err    io.Writer
}

func New(input io.Reader, output, errorOutput io.Writer) *Console {
	return &Console{reader: bufio.NewReader(input), out: output, err: errorOutput}
}

func (c *Console) Header(info session.Info) {
	fmt.Fprintf(c.out, "Ayati | fireworks/%s | session %s\n", info.Model, shortID(info.ID))
	fmt.Fprintf(c.out, "workspace: %s\n", info.Workspace)
	fmt.Fprintln(c.out, "Trusted-local mode: shell commands run with your user permissions.")
	fmt.Fprintln(c.out, "Type /help for commands or /exit to quit.")
}

func (c *Console) Prompt() (string, error) {
	fmt.Fprint(c.out, "\nyou> ")
	var value strings.Builder
	for {
		part, prefix, err := c.reader.ReadLine()
		if err != nil {
			return "", err
		}
		if value.Len()+len(part) > maxInputBytes {
			if prefix {
				c.discardLine()
			}
			return "", ErrInputTooLarge
		}
		value.Write(part)
		if !prefix {
			return value.String(), nil
		}
	}
}

func (c *Console) Help() {
	fmt.Fprintln(c.out, "Commands:")
	fmt.Fprintln(c.out, "  /new          start an empty session")
	fmt.Fprintln(c.out, "  /sessions     list sessions for this workspace")
	fmt.Fprintln(c.out, "  /resume <id>  resume a session by id or unique prefix")
	fmt.Fprintln(c.out, "  /help         show this help")
	fmt.Fprintln(c.out, "  /exit         quit")
}

func (c *Console) Sessions(infos []session.Info, activeID string) {
	if len(infos) == 0 {
		fmt.Fprintln(c.out, "session> none found")
		return
	}
	for _, info := range infos {
		marker := " "
		if info.ID == activeID {
			marker = "*"
		}
		fmt.Fprintf(c.out, "%s %s  %s  %s\n", marker, shortID(info.ID), info.Model, info.UpdatedAt.Local().Format("2006-01-02 15:04"))
	}
}

func (c *Console) SessionActivated(info session.Info) {
	fmt.Fprintf(c.out, "session> active %s (%s)\n", shortID(info.ID), info.Model)
}

func (c *Console) Step(current, maximum int) {
	fmt.Fprintf(c.out, "run> step %d/%d\n", current, maximum)
}

func (c *Console) ToolCall(command string) {
	fmt.Fprintf(c.out, "shell> %s\n", command)
}

func (c *Console) ToolResult(result agent.ShellResult) {
	if result.Stdout != "" {
		fmt.Fprintf(c.out, "stdout>\n%s", result.Stdout)
		if !strings.HasSuffix(result.Stdout, "\n") {
			fmt.Fprintln(c.out)
		}
	}
	if result.Stderr != "" {
		fmt.Fprintf(c.out, "stderr>\n%s", result.Stderr)
		if !strings.HasSuffix(result.Stderr, "\n") {
			fmt.Fprintln(c.out)
		}
	}
	duration := result.Duration.Round(time.Millisecond)
	fmt.Fprintf(c.out, "result> exit=%d duration=%s", result.ExitCode, duration)
	if result.TimedOut {
		fmt.Fprint(c.out, " timed-out")
	}
	if result.Truncated {
		fmt.Fprint(c.out, " truncated")
	}
	if result.Error != "" {
		fmt.Fprintf(c.out, " error=%s", result.Error)
	}
	fmt.Fprintln(c.out)
}

func (c *Console) Assistant(text string) {
	fmt.Fprintf(c.out, "assistant> %s\n", text)
}

func (c *Console) Error(err error) {
	fmt.Fprintf(c.err, "ayati: %v\n", err)
}

func (c *Console) Newline() {
	fmt.Fprintln(c.out)
}

func (c *Console) discardLine() {
	for {
		_, prefix, err := c.reader.ReadLine()
		if err != nil || !prefix {
			return
		}
	}
}

func shortID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}
