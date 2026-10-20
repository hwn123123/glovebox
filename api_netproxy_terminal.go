package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const netproxyTerminalLimit = 32 << 10

var netproxyTerminalCommands = map[string]bool{
	"api": true, "app": true, "catalog": true, "config": true,
	"ebpf": true, "logs": true, "mode": true, "network": true,
	"node": true, "service": true, "sub": true,
}

// splitNetproxyCommand tokenizes the quoting emitted by the upstream terminal.
// It intentionally implements no substitutions, redirects, pipes, or command
// separators, so browser input can never become a root shell command.
func splitNetproxyCommand(command string) ([]string, error) {
	if len(command) == 0 || len(command) > netproxyTerminalLimit {
		return nil, apiErr("terminal_command_invalid", "NetProxy command is empty or too long")
	}
	var words []string
	var word strings.Builder
	inWord := false
	quote := byte(0)
	escaped := false
	flush := func() {
		if inWord {
			words = append(words, word.String())
			word.Reset()
			inWord = false
		}
	}
	for i := 0; i < len(command); i++ {
		c := command[i]
		if c == 0 || c == '\n' || c == '\r' {
			return nil, apiErr("terminal_command_invalid", "NetProxy command contains an invalid character")
		}
		if escaped {
			word.WriteByte(c)
			inWord = true
			escaped = false
			continue
		}
		switch quote {
		case '\'':
			if c == '\'' {
				quote = 0
			} else {
				word.WriteByte(c)
			}
			inWord = true
		case '"':
			if c == '"' {
				quote = 0
			} else if c == '\\' {
				escaped = true
			} else {
				word.WriteByte(c)
			}
			inWord = true
		default:
			switch c {
			case '\'', '"':
				quote = c
				inWord = true
			case '\\':
				escaped = true
				inWord = true
			case ';', '|', '&', '<', '>':
				return nil, apiErr("terminal_command_rejected", "shell operators are not available in the LAN terminal")
			case ' ', '\t':
				flush()
			default:
				word.WriteByte(c)
				inWord = true
			}
		}
	}
	if quote != 0 || escaped {
		return nil, apiErr("terminal_command_invalid", "NetProxy command has unfinished quoting")
	}
	flush()
	if len(words) < 2 || len(words) > 128 || filepath.Base(words[0]) != "netproxyctl" {
		return nil, apiErr("terminal_command_rejected", "only netproxyctl commands are available in the LAN terminal")
	}
	return words, nil
}

func validateNetproxyTerminalArgs(args []string) error {
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		switch args[0] {
		case "--json":
			args = args[1:]
		case "--timeout":
			if len(args) < 2 || strings.HasPrefix(args[1], "-") {
				return apiErr("terminal_command_invalid", "--timeout requires a duration")
			}
			args = args[2:]
		default:
			return apiErr("terminal_command_rejected", "unsupported NetProxy global option")
		}
	}
	if len(args) == 0 || !netproxyTerminalCommands[args[0]] {
		return apiErr("terminal_command_rejected", "unsupported NetProxy command")
	}
	return nil
}

func (s *server) handleNetProxyTerminal(w http.ResponseWriter, r *http.Request) {
	if s.platform != "xiaomi" {
		writeErr(w, http.StatusNotFound, apiErr("terminal_unavailable", "NetProxy is not installed on this platform"))
		return
	}
	var in struct {
		Command string `json:"command"`
	}
	if err := readBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	words, err := splitNetproxyCommand(in.Command)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	args := words[1:]
	if err := validateNetproxyTerminalArgs(args); err != nil {
		writeErr(w, http.StatusForbidden, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, netproxyExecutable(s.cfg.get()), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() != nil {
			exitCode = 124
			if stderr.Len() == 0 {
				stderr.WriteString("NetProxy command timed out")
			}
		} else if stderr.Len() == 0 {
			stderr.WriteString(runErr.Error())
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"errno": exitCode, "stdout": stdout.String(), "stderr": stderr.String(),
	})
}
