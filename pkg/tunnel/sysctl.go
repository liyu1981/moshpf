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
	msg := fmt.Sprintf("\nWARNING: UDP buffer size on %s side is below optimal for QUIC.\n", side)
	msg += fmt.Sprintf("Current: rmem_max=%d, wmem_max=%d\n", info.RMemMax, info.WMemMax)
	msg += fmt.Sprintf("Recommended: at least %d\n", MinBufferBytes)
	msg += "To fix this, run the following commands with sudo:\n"
	msg += fmt.Sprintf("  sudo sysctl -w net.core.rmem_max=%d\n", MinBufferBytes)
	msg += fmt.Sprintf("  sudo sysctl -w net.core.wmem_max=%d\n", MinBufferBytes)
	msg += "Documentation: https://github.com/liyu1981/moshpf/wiki/quic-buffer.md\n"
	msg += "Press Enter to continue anyway, or Esc to exit..."
	return msg
}
