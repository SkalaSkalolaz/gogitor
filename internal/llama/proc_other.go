//go:build !linux

package llama

import "syscall"

// procAttr на прочих ОС ограничивается новой группой процессов.
func procAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}