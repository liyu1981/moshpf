package constant

import (
	"fmt"
)

const (
	M      = "\033[35mm\033[0m" // Magenta
	P      = "\033[36mp\033[0m" // Cyan
	F      = "\033[32mf\033[0m" // Green
	MPF    = M + P + F
	Github = "https://github.com/liyu1981/moshpf"
)

func GetAppLine() string {
	return fmt.Sprintf("%s (%sosh %sort %sorward) - %s", MPF, M, P, F, Github)

}
