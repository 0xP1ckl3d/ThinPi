package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"thinpi.local/agent/internal/config"
)

type Manifest struct {
	TicketID          int64           `json:"ticket_id"`
	ConnectionID      int64           `json:"connection_id"`
	Name              string          `json:"name"`
	Protocol          string          `json:"protocol"`
	Host              string          `json:"host"`
	Port              int             `json:"port"`
	Username          string          `json:"username,omitempty"`
	Password          string          `json:"password,omitempty"`
	CredentialType    string          `json:"credential_type,omitempty"`
	Config            json.RawMessage `json:"config"`
	MaxSessionSeconds int             `json:"max_session_seconds,omitempty"`
}
type Client struct {
	base  *url.URL
	token string
	http  *http.Client
}

func New(controllerURL, deviceToken, caFile string) (*Client, error) {
	base, err := url.Parse(controllerURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, errors.New("invalid controller URL")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, err
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("CA file contains no certificates")
		}
		transport.TLSClientConfig.RootCAs = pool
	}
	return &Client{base: base, token: deviceToken, http: &http.Client{Transport: transport, Timeout: 20 * time.Second}}, nil
}
func (c *Client) request(ctx context.Context, method, path string, body, output any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	u := c.base.ResolveReference(&url.URL{Path: path})
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	limited := io.LimitReader(res.Body, 1<<20)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var e struct {
			Error struct{ Code, Message string }
		}
		_ = json.NewDecoder(limited).Decode(&e)
		if e.Error.Message == "" {
			e.Error.Message = res.Status
		}
		return fmt.Errorf("controller: %s: %s", e.Error.Code, e.Error.Message)
	}
	if output != nil && res.StatusCode != 204 {
		return json.NewDecoder(limited).Decode(output)
	}
	return nil
}
func (c *Client) Redeem(ctx context.Context, ticket string) (Manifest, error) {
	var out struct {
		Manifest Manifest `json:"manifest"`
	}
	err := c.request(ctx, "POST", "/api/v1/agent/redeem-launch", map[string]string{"ticket": ticket}, &out)
	return out.Manifest, err
}
func (c *Client) RedeemMaintenance(ctx context.Context, ticket string) error {
	return c.request(ctx, "POST", "/api/v1/agent/redeem-maintenance", map[string]string{"ticket": ticket}, nil)
}
func (c *Client) Heartbeat(ctx context.Context, versions any) error {
	return c.request(ctx, "POST", "/api/v1/agent/heartbeat", map[string]any{"versions": versions}, nil)
}
func (c *Client) SessionEvent(ctx context.Context, ticketID, connectionID int64, event, result string, metadata any) error {
	return c.request(ctx, "POST", "/api/v1/agent/session-event", map[string]any{"ticket_id": ticketID, "connection_id": connectionID, "event": event, "result": result, "metadata": metadata}, nil)
}
func Enrol(ctx context.Context, controllerURL, caFile, token, identifier, name string) (config.DeviceFile, error) {
	c, err := New(controllerURL, "", caFile)
	if err != nil {
		return config.DeviceFile{}, err
	}
	var out struct {
		Device struct {
			DeviceIdentifier string `json:"device_identifier"`
			Name             string `json:"name"`
		} `json:"device"`
		DeviceToken string `json:"device_token"`
	}
	err = c.request(ctx, "POST", "/api/v1/devices/enrol", map[string]string{"token": strings.TrimSpace(token), "device_identifier": identifier, "name": name}, &out)
	return config.DeviceFile{DeviceIdentifier: out.Device.DeviceIdentifier, DeviceToken: out.DeviceToken, Name: out.Device.Name}, err
}
