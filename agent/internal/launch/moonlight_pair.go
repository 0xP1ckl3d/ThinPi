package launch

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	moonlightPairTimeout = 30 * time.Second
	moonlightListTimeout = 8 * time.Second
)

type sunshineCertificateStore struct {
	path string
	mu   sync.Mutex
}

type sunshinePINResponse struct {
	Status bool   `json:"status"`
	Error  string `json:"error"`
}

type sunshineCertificateChangedError struct{ target string }

func (e *sunshineCertificateChangedError) Error() string {
	return fmt.Sprintf("Sunshine HTTPS certificate changed for %s", e.target)
}

func ensureMoonlightPaired(ctx context.Context, command Command, configure func(*exec.Cmd)) error {
	pairing := command.MoonlightPairing
	if pairing == nil {
		return nil
	}

	listCtx, cancelList := context.WithTimeout(ctx, moonlightListTimeout)
	listCommand := exec.CommandContext(listCtx, command.Path, "list", pairing.Host)
	configure(listCommand)
	if err := listCommand.Run(); err == nil {
		cancelList()
		return nil
	}
	cancelList()

	if pairing.Username == "" || pairing.Password == "" {
		return &ClientRuntimeError{Message: "Moonlight is not paired with this Sunshine host. Assign its Sunshine Web UI administrator credential to this connection and try again."}
	}

	pin, err := randomMoonlightPIN()
	if err != nil {
		return errors.New("could not generate a Moonlight pairing PIN")
	}
	pairCtx, cancelPair := context.WithTimeout(ctx, moonlightPairTimeout)
	defer cancelPair()
	pairCommand := exec.CommandContext(pairCtx, command.Path, "pair", pairing.Host, "--pin", pin)
	pairOutput := &boundedOutput{limit: 32768}
	pairCommand.Stdout = pairOutput
	pairCommand.Stderr = pairOutput
	configure(pairCommand)
	if err = pairCommand.Start(); err != nil {
		return &ClientRuntimeError{Message: "Moonlight pairing could not be started.", Diagnostic: err.Error()}
	}

	wait := make(chan error, 1)
	go func() { wait <- pairCommand.Wait() }()

	certificateTarget := net.JoinHostPort(pairing.Host, fmt.Sprint(pairing.SunshineAPIPort))
	client := newSunshinePairingClient(defaultSunshineCertificatesPath(), certificateTarget)
	defer client.CloseIdleConnections()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var lastAPIError error
	approved := false
	for {
		select {
		case pairErr := <-wait:
			if pairErr == nil {
				return nil
			}
			diagnostic := redactClientOutput(pairOutput.String(), command)
			if diagnostic == "" {
				diagnostic = pairErr.Error()
			}
			return &ClientRuntimeError{Message: "Moonlight could not complete automatic pairing with Sunshine.", Diagnostic: diagnostic}
		case <-pairCtx.Done():
			_ = pairCommand.Process.Kill()
			<-wait
			if ctx.Err() != nil {
				return ctx.Err()
			}
			diagnostic := redactClientOutput(pairOutput.String(), command)
			if lastAPIError != nil {
				diagnostic = strings.TrimSpace(diagnostic + " " + lastAPIError.Error())
			}
			return &ClientRuntimeError{Message: "Automatic Moonlight pairing timed out. Check the Sunshine host, Web UI credentials, and remote PIN access setting.", Diagnostic: diagnostic}
		case <-ticker.C:
			if approved {
				continue
			}
			accepted, permanent, submitErr := submitSunshinePIN(pairCtx, client, *pairing, pin)
			if submitErr != nil {
				lastAPIError = submitErr
				if permanent {
					cancelPair()
					<-wait
					return submitErr
				}
				continue
			}
			approved = accepted
		}
	}
}

func randomMoonlightPIN() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%04d", value.Int64()), nil
}

func submitSunshinePIN(ctx context.Context, client *http.Client, pairing MoonlightPairing, pin string) (bool, bool, error) {
	body, err := json.Marshal(map[string]string{"pin": pin, "name": pairing.ClientName})
	if err != nil {
		return false, true, err
	}
	target := net.JoinHostPort(pairing.Host, fmt.Sprint(pairing.SunshineAPIPort))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+target+"/api/pin", bytes.NewReader(body))
	if err != nil {
		return false, true, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(pairing.Username, pairing.Password)
	response, err := client.Do(request)
	if err != nil {
		var changed *sunshineCertificateChangedError
		if errors.As(err, &changed) {
			return false, true, &ClientRuntimeError{Message: "Sunshine's Web UI certificate changed. Verify the Sunshine host before pairing again.", Diagnostic: changed.Error()}
		}
		return false, false, err
	}
	defer response.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
	if readErr != nil {
		return false, false, readErr
	}
	if response.StatusCode == http.StatusUnauthorized {
		return false, true, &ClientRuntimeError{Message: "Sunshine rejected the stored Web UI administrator username or password."}
	}
	if response.StatusCode == http.StatusForbidden {
		return false, true, &ClientRuntimeError{Message: "Sunshine blocked remote PIN submission. Allow PIN entry from the ThinPi network in Sunshine and try again."}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, false, fmt.Errorf("Sunshine PIN API returned HTTP %d", response.StatusCode)
	}
	var result sunshinePINResponse
	if err = json.Unmarshal(responseBody, &result); err != nil {
		return false, false, errors.New("Sunshine PIN API returned an invalid response")
	}
	if result.Error != "" {
		return false, true, &ClientRuntimeError{Message: "Sunshine rejected automatic Moonlight pairing.", Diagnostic: result.Error}
	}
	return result.Status, false, nil
}

func newSunshinePairingClient(certificatePath, certificateTarget string) *http.Client {
	store := &sunshineCertificateStore{path: certificatePath}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// Sunshine uses a self-signed Web UI certificate by default. Verification
			// is performed below against ThinPi's persisted TOFU fingerprint instead.
			InsecureSkipVerify: true, //nolint:gosec
			VerifyConnection: func(state tls.ConnectionState) error {
				if len(state.PeerCertificates) == 0 {
					return errors.New("Sunshine returned no HTTPS certificate")
				}
				return store.verify(certificateTarget, state.PeerCertificates[0].Raw)
			},
		},
	}
	return &http.Client{Transport: transport, Timeout: 3 * time.Second}
}

func (s *sunshineCertificateStore) verify(target string, certificate []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	digest := sha256.Sum256(certificate)
	fingerprint := hex.EncodeToString(digest[:])
	trusted := map[string]string{}
	content, err := os.ReadFile(s.path)
	if err == nil {
		if err = json.Unmarshal(content, &trusted); err != nil {
			return errors.New("stored Sunshine certificate trust is invalid")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if existing := trusted[target]; existing != "" {
		if existing != fingerprint {
			return &sunshineCertificateChangedError{target: target}
		}
		return nil
	}
	trusted[target] = fingerprint
	return writeSunshineCertificates(s.path, trusted)
}

func writeSunshineCertificates(path string, trusted map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	content, err := json.Marshal(trusted)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".sunshine-certificates-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err = temp.Chmod(0640); err == nil {
		_, err = temp.Write(append(content, '\n'))
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func defaultSunshineCertificatesPath() string {
	if configured := os.Getenv("THINPI_SUNSHINE_CERTIFICATES"); configured != "" {
		return configured
	}
	return "/var/lib/thinpi-agent/sunshine-certificates.json"
}
