package cli

import (
	"fmt"
	"strings"
)

// Platform-neutral parsers for process inspection output. They live here
// (not in the build-tagged files) so they are unit-tested on every OS.

// parseProcStat parses /proc/<pid>/stat. It returns the command name
// (field 2), whether the process is a zombie (field 3 == 'Z'), and the
// start time in clock ticks since boot (field 22).
//
// The command name is wrapped in parentheses and may itself contain
// spaces and ')' characters, so the fields after it are located from the
// LAST ')' rather than by splitting the whole line.
func parseProcStat(stat string) (name string, zombie bool, startTime string, err error) {
	open := strings.IndexByte(stat, '(')
	end := strings.LastIndexByte(stat, ')')
	if open < 0 || end < open {
		return "", false, "", fmt.Errorf("malformed /proc stat line")
	}
	name = stat[open+1 : end]
	rest := strings.Fields(stat[end+1:])
	// rest[0] is field 3 (state); field 22 (starttime) is rest[19].
	if len(rest) < 20 {
		return "", false, "", fmt.Errorf("short /proc stat line (%d fields after name)", len(rest))
	}
	return name, rest[0] == "Z", rest[19], nil
}

// psColumns is the column spec passed to `ps -o`. comm comes last because
// on macOS it is the executable path, which may contain spaces.
const psColumns = "state=,lstart=,comm="

// parsePSLine parses one line of `ps -o state=,lstart=,comm=` (run with
// LC_ALL=C). lstart is always five tokens ("Fri Oct  2 10:40:03 2026").
func parsePSLine(line string) (state, lstart, comm string, err error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", "", fmt.Errorf("empty ps output")
	}
	var tokens []string
	rest := line
	for len(tokens) < 6 {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			return "", "", "", fmt.Errorf("short ps output %q", line)
		}
		i := strings.IndexAny(rest, " \t")
		if i < 0 {
			tokens = append(tokens, rest)
			rest = ""
			continue
		}
		tokens = append(tokens, rest[:i])
		rest = rest[i:]
	}
	comm = strings.TrimSpace(rest)
	if comm == "" {
		return "", "", "", fmt.Errorf("ps output has no command: %q", line)
	}
	return tokens[0], strings.Join(tokens[1:6], " "), comm, nil
}
