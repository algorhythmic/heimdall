package nativebridge

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"heimdall/internal/browser"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type shortWriter struct{ bytes.Buffer }

func (w *shortWriter) Write(b []byte) (int, error) {
	if len(b) > 2 {
		b = b[:2]
	}
	return w.Buffer.Write(b)
}
func TestFrames(t *testing.T) {
	w := &shortWriter{}
	body := []byte(`{"unicode":"日"}`)
	if e := WriteFrame(w, body); e != nil {
		t.Fatal(e)
	}
	got, e := ReadFrame(w)
	if e != nil || !bytes.Equal(body, got) {
		t.Fatal(e)
	}
	for _, n := range []uint32{0, browser.MaxFrame + 1} {
		var h [4]byte
		binary.NativeEndian.PutUint32(h[:], n)
		if _, e := ReadFrame(bytes.NewReader(h[:])); e == nil {
			t.Fatal("length accepted")
		}
	}
	if _, e := ReadFrame(bytes.NewReader([]byte{1, 0})); e != io.ErrUnexpectedEOF {
		t.Fatal(e)
	}
}
func TestNativeRoleProxyAndOrigin(t *testing.T) {
	dir := t.TempDir()
	token := strings.Repeat("a", 64)
	seen := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen++
		if r.URL.Path != "/browser/message" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("incorrect route/role")
		}
		json.NewEncoder(w).Encode(browser.Reply{V: 1, Type: "ack", ID: strings.Repeat("b", 32)})
	}))
	defer server.Close()
	ep, _ := json.Marshal(Endpoint{URL: server.URL, Token: token})
	os.WriteFile(filepath.Join(dir, "browser-endpoint.json"), ep, 0600)
	m := browser.Message{V: 1, Type: "hello", ID: strings.Repeat("b", 32), Profile: strings.Repeat("c", 32), Epoch: strings.Repeat("d", 32), Connection: strings.Repeat("e", 32), ExtensionVersion: "0.2.0"}
	b, _ := json.Marshal(m)
	var in, out bytes.Buffer
	WriteFrame(&in, b)
	c := Config{DataDir: dir, ExtensionID: strings.Repeat("a", 32)}
	if e := Run(context.Background(), c, "chrome-extension://"+c.ExtensionID+"/", &in, &out); e != nil {
		t.Fatal(e)
	}
	reply, e := ReadFrame(&out)
	if e != nil || !bytes.Contains(reply, []byte(`"ack"`)) || seen != 1 {
		t.Fatal(string(reply), e, seen)
	}
	if e := Run(context.Background(), c, "chrome-extension://"+strings.Repeat("b", 32)+"/", bytes.NewReader(nil), &out); e == nil {
		t.Fatal("foreign origin accepted")
	}
}
