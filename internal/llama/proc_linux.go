//go:build linux

package llama

import "syscall"

// procAttr на Linux дополнительно устанавливает Pdeathsig, чтобы
// llama-server автоматически получал SIGTERM, если Gogitor падает
// аварийно (например, при SIGKILL самого Gogitor).
func procAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
}