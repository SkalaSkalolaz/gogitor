package computer

import (
	"testing"
)

func TestDetectShellName(t *testing.T) {
	if detectShellName() == "" {
		t.Error("empty shell")
	}
}

func TestCmdExists(t *testing.T) {
	if cmdExists("definitely_not_a_real_command_12345") {
		t.Error("should not exist")
	}
}

func TestOSInfo_InstallCmd(t *testing.T) {
	for _, tc := range []struct{ pm, pkg, want string }{
		{"apt", "curl", "apt install -y curl"},
		{"dnf", "curl", "dnf install -y curl"},
		{"pacman", "curl", "pacman -S --noconfirm curl"},
		{"apk", "curl", "apk add curl"},
		{"brew", "curl", "brew install curl"},
		{"winget", "curl", "winget install curl"},
		{"unknown", "curl", "echo 'unknown package manager'"},
	} {
		info := OSInfo{PkgManager: tc.pm}
		if got := info.InstallCmd(tc.pkg); got != tc.want {
			t.Errorf("InstallCmd(%q) = %q, want %q", tc.pkg, got, tc.want)
		}
	}
}