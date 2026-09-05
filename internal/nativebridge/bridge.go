// Package nativebridge is a framed browser-only proxy, never a database writer.
package nativebridge

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"heimdall/internal/browser"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DataDir     string `json:"data_dir"`
	ExtensionID string `json:"extension_id"`
}
type Endpoint struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

func ReadFrame(r io.Reader) ([]byte, error) {
	var head [4]byte
	if _, e := io.ReadFull(r, head[:]); e != nil {
		return nil, e
	}
	n := binary.NativeEndian.Uint32(head[:])
	if n == 0 || n > browser.MaxFrame {
		return nil, fmt.Errorf("native frame length out of bounds")
	}
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	return b, err
}
func WriteFrame(w io.Writer, b []byte) error {
	if len(b) == 0 || len(b) > browser.MaxFrame {
		return fmt.Errorf("native output frame too large")
	}
	var head [4]byte
	binary.NativeEndian.PutUint32(head[:], uint32(len(b)))
	for _, part := range [][]byte{head[:], b} {
		for len(part) > 0 {
			n, err := w.Write(part)
			if err != nil {
				return err
			}
			if n == 0 {
				return io.ErrShortWrite
			}
			part = part[n:]
		}
	}
	return nil
}
func Proxy(ctx context.Context, dir string, m browser.Message) ([]byte, error) {
	file, err := os.ReadFile(filepath.Join(dir, "browser-endpoint.json"))
	if err != nil {
		return nil, fmt.Errorf("daemon unavailable: %w", err)
	}
	var ep Endpoint
	if err = json.Unmarshal(file, &ep); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(ep.URL, "http://127.0.0.1:") {
		return nil, fmt.Errorf("invalid browser endpoint")
	}
	port, err := strconv.Atoi(strings.TrimPrefix(ep.URL, "http://127.0.0.1:"))
	if err != nil || port < 1 || port > 65535 || len(ep.Token) != 64 {
		return nil, fmt.Errorf("invalid browser endpoint")
	}
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	if err = encoder.Encode(m); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", ep.URL+"/browser/message", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+ep.Token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 4 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, browser.MaxFrame+1))
	if err != nil {
		return nil, err
	}
	if len(b) > browser.MaxFrame {
		return nil, fmt.Errorf("daemon response too large")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("daemon %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return b, nil
}
func Run(ctx context.Context, c Config, origin string, in io.Reader, out io.Writer) error {
	if !browser.ExtensionPattern.MatchString(c.ExtensionID) || origin != "chrome-extension://"+c.ExtensionID+"/" {
		return fmt.Errorf("native caller origin is not allowed")
	}
	if !filepath.IsAbs(c.DataDir) {
		return fmt.Errorf("native data_dir must be absolute")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		frame, err := ReadFrame(in)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		m, err := browser.Decode(frame)
		var reply []byte
		if err == nil {
			reply, err = Proxy(ctx, c.DataDir, m)
		}
		if err != nil {
			reply, _ = json.Marshal(browser.Reply{V: 1, Type: "error", ID: m.ID, Error: err.Error()})
		}
		if err = WriteFrame(out, reply); err != nil {
			return err
		}
	}
}
func Main(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	configPath := filepath.Join(filepath.Dir(exe), "host-config.json")
	origin := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--config" {
			i++
			if i >= len(args) {
				return fmt.Errorf("--config requires path")
			}
			configPath = args[i]
		} else if strings.HasPrefix(a, "chrome-extension://") {
			if origin != "" {
				return fmt.Errorf("multiple origins")
			}
			origin = a
		} else if !strings.HasPrefix(a, "--parent-window=") {
			return fmt.Errorf("unexpected native argument")
		}
	}
	b, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var c Config
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return err
	}
	return Run(ctx, c, origin, in, out)
}
