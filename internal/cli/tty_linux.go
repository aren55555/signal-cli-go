package cli

import (
	"os"
	"syscall"
	"unsafe"
)

// ttyColumns asks the controlling terminal for its width, as the
// reference does via /dev/tty; 0 if there is none.
func ttyColumns() int {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return 0
	}
	defer f.Close()
	var ws struct{ rows, cols, x, y uint16 }
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws))); e != 0 {
		return 0
	}
	return int(ws.cols)
}
