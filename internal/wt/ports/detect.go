package ports

import (
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

// Listeners returns LISTEN sockets whose local port falls inside the dev band,
// keyed by port. It shells out to `ss -tlnpH`; if ss is unavailable the result
// is an empty map (callers degrade to "all down" rather than failing).
func Listeners() (map[int]Listener, error) {
	out, err := exec.Command("ss", "-tlnpH").Output()
	if err != nil {
		return map[int]Listener{}, nil //nolint:nilerr // ss missing → treat as no listeners
	}
	return parseSS(string(out)), nil
}

// parseSS parses the output of `ss -tlnpH`, keeping only ports in the dev band.
func parseSS(out string) map[int]Listener {
	res := map[int]Listener{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		port, ok := portFromLocalAddr(fields[3])
		if !ok || port < BandStart || port > BandEnd {
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
