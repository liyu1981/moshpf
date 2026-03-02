package util

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// SelectMenu displays a selectable menu in the terminal and returns the index of the selected option.
// Returns -1 and an error if aborted by user (Esc or Ctrl+C).
func SelectMenu(prompt string, options []string) (int, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return 0, nil // Default to first option if not a terminal
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return -1, err
	}
	defer term.Restore(fd, oldState)

	selected := 0

	renderMenu := func() {
		fmt.Printf("\r%s\r\n", prompt)
		for i, opt := range options {
			if i == selected {
				fmt.Printf("\r\033[2K \033[1;34m>\033[0m \033[1m%s\033[0m\r\n", opt)
			} else {
				fmt.Printf("\r\033[2K   %s\r\n", opt)
			}
		}
		fmt.Printf("\033[%dA", len(options)+1) // Move cursor back up (including prompt)
	}

	renderMenu()

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			fmt.Printf("\033[%dB\r\n", len(options)+1)
			return -1, err
		}

		if n == 1 {
			if buf[0] == '\r' || buf[0] == '\n' {
				// Confirm selection
				fmt.Printf("\033[%dB\r\n", len(options)+1)
				return selected, nil
			}
			if buf[0] == 0x1b { // Esc
				fmt.Printf("\033[%dB\r\n", len(options)+1)
				return -1, fmt.Errorf("aborted")
			}
			if buf[0] == 3 { // Ctrl+C
				fmt.Printf("\033[%dB\r\n", len(options)+1)
				return -1, fmt.Errorf("aborted")
			}
		} else if n == 3 && buf[0] == 0x1b && buf[1] == '[' {
			if buf[2] == 'A' { // Up
				if selected > 0 {
					selected--
					renderMenu()
				}
			} else if buf[2] == 'B' { // Down
				if selected < len(options)-1 {
					selected++
					renderMenu()
				}
			}
		}
	}
}
