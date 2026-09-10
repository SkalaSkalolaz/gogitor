package app

import "strings"

// normalizeTUICommand accepts both the documented :command form and the
// legacy command-without-colon spelling. It keeps command dispatch deterministic
// and prevents harmless formatting differences from becoming "unknown command".
func normalizeTUICommand(raw string) string {
	cmd := strings.ToLower(strings.TrimSpace(raw))
	if cmd == "" {
		return cmd
	}
	if !strings.HasPrefix(cmd, ":") {
		cmd = ":" + cmd
	}
	return cmd
}
