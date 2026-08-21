package computer

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// OSInfo — информация об ОС пользователя.
type OSInfo struct {
	OS         string `json:"os"`
	Distro     string `json:"distro"`
	Version    string `json:"version"`
	PkgManager string `json:"pkg_manager"`
	Shell      string `json:"shell"`
	IsRoot     bool   `json:"is_root"`
	HasSudo    bool   `json:"has_sudo"`
}

func DetectOS() OSInfo {
	info := OSInfo{OS: runtime.GOOS}
	switch runtime.GOOS {
	case "linux":
		info.detectLinux()
	case "darwin":
		info.detectDarwin()
	case "windows":
		info.detectWindows()
	}
	info.Shell = detectShellName()
	info.IsRoot = os.Getuid() == 0
	info.HasSudo = checkSudo()
	return info
}

func (i *OSInfo) detectLinux() {
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "ID=") {
				i.Distro = strings.Trim(strings.TrimPrefix(line, "ID="), `"`)
			}
			if strings.HasPrefix(line, "VERSION_ID=") {
				i.Version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), `"`)
			}
		}
	}
	switch {
	case cmdExists("apt"):
		i.PkgManager = "apt"
	case cmdExists("dnf"):
		i.PkgManager = "dnf"
	case cmdExists("pacman"):
		i.PkgManager = "pacman"
	case cmdExists("apk"):
		i.PkgManager = "apk"
	case cmdExists("zypper"):
		i.PkgManager = "zypper"
	default:
		i.PkgManager = "unknown"
	}
}

func (i *OSInfo) detectDarwin() {
	i.Distro = "macos"
	if out, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
		i.Version = strings.TrimSpace(string(out))
	}
	if cmdExists("brew") {
		i.PkgManager = "brew"
	} else {
		i.PkgManager = "none"
	}
}

func (i *OSInfo) detectWindows() {
	i.Distro = "windows"
	i.PkgManager = "winget"
	if cmdExists("apt") {
		i.PkgManager = "apt"
		i.Distro = "wsl"
	}
}

func detectShellName() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s[strings.LastIndex(s, "/")+1:]
	}
	if cmdExists("bash") {
		return "bash"
	}
	return "sh"
}

func cmdExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func checkSudo() bool {
	return exec.Command("sudo", "-n", "true").Run() == nil
}

func (i OSInfo) InstallCmd(pkg string) string {
	switch i.PkgManager {
	case "apt":
		return "apt install -y " + pkg
	case "dnf":
		return "dnf install -y " + pkg
	case "pacman":
		return "pacman -S --noconfirm " + pkg
	case "apk":
		return "apk add " + pkg
	case "brew":
		return "brew install " + pkg
	case "winget":
		return "winget install " + pkg
	default:
		return "echo 'unknown package manager'"
	}
}