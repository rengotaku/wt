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

// Listeners returns LISTEN sockets whose local port falls inside [start, end],
// keyed by port. It shells out to `ss -tlnpH`; if ss is unavailable the result
// is an empty map (callers degrade to "all down" rather than failing).
func Listeners(start, end int) (map[int]Listener, error) {
	out, err := exec.Command("ss", "-tlnpH").Output()
	if err != nil {
		return map[int]Listener{}, nil //nolint:nilerr // ss missing → treat as no listeners
	}
	return parseSS(string(out), start, end), nil
}

// AllListeners returns every LISTEN socket on the machine (no band filter),
// keyed by port. Used by the port doctor to surface foreign squatters.
func AllListeners() (map[int]Listener, error) {
	out, err := exec.Command("ss", "-tlnpH").Output()
	if err != nil {
		return map[int]Listener{}, nil //nolint:nilerr // ss missing → treat as no listeners
	}
	return parseSS(string(out), 1, 65535), nil
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
// endpoint of an ESTABLISHED TCP socket. ss 不在時は空 map(degrade, nil err)。
func Established(start, end int) (map[int]bool, error) {
	out, err := exec.Command("ss", "-tnH", "state", "established").Output()
	if err != nil {
		return map[int]bool{}, nil //nolint:nilerr // ss missing → degrade
	}
	return parseEstablished(string(out), start, end), nil
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
