package tunnel

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const MinBufferBytes = 7500000

type UDPBufferInfo struct {
	RMemMax int
	WMemMax int
}

func GetUDPBufferInfo() (UDPBufferInfo, error) {
	info := UDPBufferInfo{}

	rmem, err := getSysctlValue("net.core.rmem_max")
	if err != nil {
		return info, err
	}
	info.RMemMax = rmem

	wmem, err := getSysctlValue("net.core.wmem_max")
	if err != nil {
		return info, err
	}
	info.WMemMax = wmem

	return info, nil
}

func getSysctlValue(name string) (int, error) {
	out, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return 0, err
	}
	val, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, err
	}
	return val, nil
}

func (info UDPBufferInfo) IsOptimal() bool {
	return info.RMemMax >= MinBufferBytes && info.WMemMax >= MinBufferBytes
}

func GetBufferWarning(side string, info UDPBufferInfo) string {
	if info.IsOptimal() {
		return ""
	}
	const (
		yellow = "\033[33m"
		cyan   = "\033[36m"
		reset  = "\033[0m"
		bold   = "\033[1m"
	)

	msg := fmt.Sprintf("\r\n%s%sWARNING: UDP buffer size on %s side is below optimal for QUIC.%s\r\n", bold, yellow, side, reset)
	msg += fmt.Sprintf("Current: %srmem_max=%d, wmem_max=%d%s\r\n", cyan, info.RMemMax, info.WMemMax, reset)
	msg += fmt.Sprintf("Recommended: at least %s%d%s\r\n", bold, MinBufferBytes, reset)
	msg += fmt.Sprintf("Documentation: %shttps://github.com/liyu1981/moshpf/wiki/quic-buffer.md%s\r\n", cyan, reset)
	msg += "Press Enter to continue anyway, or Esc to exit..."
	return msg
}
