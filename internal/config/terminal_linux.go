package config

import (
	"os"
	"syscall"
	"unsafe"
)

func disableEcho(file *os.File) (func() error, error) {
	fd := file.Fd()
	var original syscall.Termios
	if err := ioctlTermios(fd, syscall.TCGETS, &original); err != nil {
		return nil, err
	}
	hidden := original
	hidden.Lflag &^= syscall.ECHO
	if err := ioctlTermios(fd, syscall.TCSETS, &hidden); err != nil {
		return nil, err
	}
	return func() error { return ioctlTermios(fd, syscall.TCSETS, &original) }, nil
}

func ioctlTermios(fd uintptr, operation uintptr, state *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, operation, uintptr(unsafe.Pointer(state)))
	if errno != 0 {
		return errno
	}
	return nil
}
