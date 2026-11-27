package ports

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Listener describes a process listening on a TCP port.
type Listener struct {
	Port int
	PID  int
	Proc string
}

var (
	ssPidRe  = regexp.MustCompile(`pid=(\d+)`)
	ssProcRe = regexp.MustCompile(`\("([^"]+)"`)
)

// Listeners returns LISTEN sockets whose local port falls inside [start, end],
// keyed by port. It shells out to `ss -tlnpH` (Linux), falling back to
// `lsof -nP -iTCP -sTCP:LISTEN -Fpcn` (macOS has no ss). If neither works the
// result is an empty map (callers degrade to "all down" rather than failing).
func Listeners(start, end int) (map[int]Listener, error) {
	return listeners(start, end), nil
}

func listeners(start, end int) map[int]Listener {
	l, _ := scanSockets(start, end)
	return l
}

// scanSockets is listeners that also reports whether a tool ran. ok is false
// when neither ss nor lsof worked; the empty map then means "unknown", not
// "nothing listens", and must not be cached (#205).
func scanSockets(start, end int) (l map[int]Listener, ok bool) {
	if out, err := exec.Command("ss", "-tlnpH").Output(); err == nil {
		return parseSS(string(out), start, end), true
	}
	if out, err := exec.Command(lsofPath(), "-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcn").Output(); err == nil {
		return parseLsof(string(out), start, end), true
	}
	return map[int]Listener{}, false
}

// parseLsof parses `lsof -Fpcn` field output: a "p<pid>" line starts a
// process, "c<command>" names it, and each "n<addr>" is one socket such as
// "*:9000", "127.0.0.1:9000" or "[::1]:9000". Keeps ports inside [start, end].
func parseLsof(out string, start, end int) map[int]Listener {
	res := map[int]Listener{}
	pid, proc := 0, ""
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
			proc = ""
		case 'c':
			proc = line[1:]
		case 'n':
			port, ok := portFromLocalAddr(line[1:])
			if !ok || port < start || port > end {
				continue
			}
			if _, exists := res[port]; !exists {
				res[port] = Listener{Port: port, PID: pid, Proc: proc}
			}
		}
	}
	return res
}

// parseSS parses `ss -tlnpH` output, keeping only ports inside [start, end].
func parseSS(out string, start, end int) map[int]Listener {
	res := map[int]Listener{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		port, ok := portFromLocalAddr(fields[3])
		if !ok || port < start || port > end {
			continue
		}
		l := Listener{Port: port}
		// The process column ("users:((...))") is the last field; match against
		// the whole line since its index varies with optional columns.
		if m := ssPidRe.FindStringSubmatch(line); m != nil {
			l.PID, _ = strconv.Atoi(m[1])
		}
		if m := ssProcRe.FindStringSubmatch(line); m != nil {
			l.Proc = m[1]
		}
		// Prefer the first listener seen for a port (deterministic).
		if _, exists := res[port]; !exists {
			res[port] = l
		}
	}
	return res
}

// portFromLocalAddr extracts the port from an ss local-address field such as
// "0.0.0.0:9000", "127.0.0.1:9000", "*:9000" or "[::]:9000".
func portFromLocalAddr(addr string) (int, bool) {
	i := strings.LastIndex(addr, ":")
	if i < 0 || i == len(addr)-1 {
		return 0, false
	}
	port, err := strconv.Atoi(addr[i+1:])
	if err != nil {
		return 0, false
	}
	return port, true
}

// Established returns band ports [start,end] that appear as the local OR peer
// endpoint of an ESTABLISHED TCP socket. It uses `ss` (Linux) and falls back to
// `lsof -Fn` (macOS has no ss; without the fallback every port looked idle and
// the idle reaper stopped in-use AutoStart worktrees, #185). If neither works
// it returns an error: "could not look" must not be read as "no connections",
// because the idle reaper stops services on that answer.
func Established(start, end int) (map[int]bool, error) {
	if out, err := exec.Command("ss", "-tnH", "state", "established").Output(); err == nil {
		return parseEstablished(string(out), start, end), nil
	}
	out, err := exec.Command(lsofPath(), "-nP", "-iTCP", "-sTCP:ESTABLISHED", "-Fn").Output()
	if err != nil {
		// lsof exits 1 when nothing matches; that is a real "no connections".
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && len(exitErr.Stderr) == 0 {
			return map[int]bool{}, nil
		}
		return nil, fmt.Errorf("接続一覧を取得できません（ss も lsof も失敗）: %w", err)
	}
	return parseLsofEstablished(string(out), start, end), nil
}

// lsofPath finds lsof even when PATH lacks /usr/sbin (e.g. a minimal
// service-manager environment on macOS).
func lsofPath() string {
	if p, err := exec.LookPath("lsof"); err == nil {
		return p
	}
	return lsofFallback
}

// lsofFallback is where macOS ships lsof. A var so tests can point it at a
// missing file.
var lsofFallback = "/usr/sbin/lsof"

// parseLsofEstablished parses `lsof -Fn` output, whose "n" lines look like
// "127.0.0.1:9000->127.0.0.1:54321", returning ports in [start, end] that
// appear on either side.
func parseLsofEstablished(out string, start, end int) map[int]bool {
	res := map[int]bool{}
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "n") {
			continue
		}
		local, peer, ok := strings.Cut(line[1:], "->")
		if !ok {
			continue
		}
		for _, addr := range []string{local, peer} {
			if port, ok := portFromLocalAddr(addr); ok && port >= start && port <= end {
				res[port] = true
			}
		}
	}
	return res
}

// parseEstablished parses `ss -tnH state established` output, returning ports
// in [start, end] that appear as either local or peer endpoints.
func parseEstablished(out string, start, end int) map[int]bool {
	res := map[int]bool{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		n := len(fields)
		if n < 2 {
			continue
		}
		local := fields[n-2]
		peer := fields[n-1]
		if port, ok := portFromLocalAddr(local); ok && port >= start && port <= end {
			res[port] = true
		}
		if port, ok := portFromLocalAddr(peer); ok && port >= start && port <= end {
			res[port] = true
		}
	}
	return res
}
