package launch

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type SSHTrustStore struct {
	Path string
	Scan func(context.Context, string, int) ([]string, error)
}

type PendingSSHHostKey struct {
	Host  string
	Port  int
	Lines []string
}

type SSHHostKeyChangedError struct {
	Host        string
	Port        int
	Fingerprint string
}

func (e *SSHHostKeyChangedError) Error() string {
	return fmt.Sprintf("The SSH host key for %s changed. Verify the new fingerprint %s before trusting it.", sshKnownHost(e.Host, e.Port), e.Fingerprint)
}

func defaultSSHKnownHostsPath() string {
	if configured := os.Getenv("THINPI_SSH_KNOWN_HOSTS"); configured != "" {
		return configured
	}
	if runtime.GOOS == "windows" {
		if root, err := os.UserConfigDir(); err == nil {
			return filepath.Join(root, "ThinPi", "ssh-known-hosts")
		}
	}
	return "/var/lib/thinpi-agent/ssh-known-hosts"
}

func NewSSHTrustStore(path string) *SSHTrustStore {
	return &SSHTrustStore{Path: path, Scan: scanSSHHostKeys}
}

func (s *SSHTrustStore) Prepare(ctx context.Context, host string, port int) (*PendingSSHHostKey, error) {
	if err := validateHost(host, port); err != nil {
		return nil, err
	}
	keys, err := s.Scan(ctx, host, port)
	if err != nil {
		return nil, err
	}
	target := sshKnownHost(host, port)
	newLines := make([]string, 0, len(keys))
	for _, key := range keys {
		newLines = append(newLines, target+" "+key)
	}
	existing, err := s.readLines()
	if err != nil {
		return nil, err
	}
	current := matchingSSHHostLines(existing, target)
	if len(current) == 0 {
		return nil, s.writeLines(append(existing, newLines...))
	}
	if equalStringSets(current, newLines) {
		return nil, nil
	}
	pending := &PendingSSHHostKey{Host: host, Port: port, Lines: newLines}
	return pending, &SSHHostKeyChangedError{Host: host, Port: port, Fingerprint: sshKeyFingerprint(keys[0])}
}

func (s *SSHTrustStore) Accept(pending *PendingSSHHostKey) error {
	if pending == nil || len(pending.Lines) == 0 {
		return errors.New("invalid pending SSH host key")
	}
	existing, err := s.readLines()
	if err != nil {
		return err
	}
	target := sshKnownHost(pending.Host, pending.Port)
	kept := existing[:0]
	for _, line := range existing {
		if !sshLineMatchesHost(line, target) {
			kept = append(kept, line)
		}
	}
	return s.writeLines(append(kept, pending.Lines...))
}

func (s *SSHTrustStore) readLines() ([]string, error) {
	file, err := os.Open(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func (s *SSHTrustStore) writeLines(lines []string) error {
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".ssh-known-hosts-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err = temp.Chmod(0640); err == nil {
		_, err = temp.WriteString(strings.Join(lines, "\n") + "\n")
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tempPath, s.Path)
}

func scanSSHHostKeys(ctx context.Context, host string, port int) ([]string, error) {
	binary, err := exec.LookPath("ssh-keyscan")
	if err != nil {
		return nil, errors.New("ssh-keyscan is unavailable")
	}
	output, err := exec.CommandContext(ctx, binary, "-T", "5", "-p", strconv.Itoa(port), host).Output()
	if err != nil && len(output) == 0 {
		return nil, errors.New("unable to collect the SSH host key")
	}
	seen := map[string]bool{}
	var keys []string
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || strings.HasPrefix(line, "#") || !validSSHKeyType(fields[1]) {
			continue
		}
		decoded, decodeErr := base64.StdEncoding.DecodeString(fields[2])
		if decodeErr != nil || len(decoded) < 32 {
			continue
		}
		key := fields[1] + " " + fields[2]
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("the SSH server returned no usable host key")
	}
	sort.Strings(keys)
	return keys, nil
}

func validSSHKeyType(value string) bool {
	return strings.HasPrefix(value, "ssh-") || strings.HasPrefix(value, "ecdsa-") || strings.HasPrefix(value, "sk-")
}

func sshKnownHost(host string, port int) string {
	if port == 22 {
		return host
	}
	return "[" + host + "]:" + strconv.Itoa(port)
}

func matchingSSHHostLines(lines []string, target string) []string {
	var matches []string
	for _, line := range lines {
		if sshLineMatchesHost(line, target) {
			matches = append(matches, line)
		}
	}
	return matches
}

func sshLineMatchesHost(line, target string) bool {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return false
	}
	for _, host := range strings.Split(fields[0], ",") {
		if host == target {
			return true
		}
	}
	return false
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa, bb := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

func sshKeyFingerprint(key string) string {
	fields := strings.Fields(key)
	if len(fields) != 2 {
		return "unknown"
	}
	decoded, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return "unknown"
	}
	digest := sha256.Sum256(decoded)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:])
}
