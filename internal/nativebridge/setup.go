package nativebridge

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"heimdall/internal/browser"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"
)

// Prepare emits inspectable artifacts. It never changes browser registry entries.
func Prepare(dataDir, extensionID, output, executable string) (map[string]string, error) {
	if !browser.ExtensionPattern.MatchString(extensionID) {
		return nil, fmt.Errorf("extension ID must be 32 characters a-p")
	}
	dataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return nil, err
	}
	if entries, err := os.ReadDir(output); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("setup output must be empty; prepare a new directory for upgrades")
	}
	if err = os.MkdirAll(output, 0700); err != nil {
		return nil, err
	}
	name := "heimdall-browser-host"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	host := filepath.Join(output, name)
	exe, err := os.ReadFile(executable)
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(host, exe, 0700); err != nil {
		return nil, err
	}
	writeJSON := func(name string, v any) error {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(output, name), b, 0600)
	}
	if err = writeJSON("host-config.json", Config{DataDir: dataDir, ExtensionID: extensionID}); err != nil {
		return nil, err
	}
	manifestName := browser.HostName + ".json"
	manifest := map[string]any{"name": browser.HostName, "description": "Heimdall browser bridge", "path": host, "type": "stdio", "allowed_origins": []string{"chrome-extension://" + extensionID + "/"}}
	if err = writeJSON(manifestName, manifest); err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(output, manifestName)
	if runtime.GOOS == "windows" {
		escaped := strings.ReplaceAll(strings.ReplaceAll(manifestPath, "\\", "\\\\"), "\"", "\\\"")
		for _, vendor := range []struct{ name, path string }{{"chrome", `Google\Chrome`}, {"edge", `Microsoft\Edge`}, {"chromium", `Chromium`}} {
			text := "Windows Registry Editor Version 5.00\r\n\r\n[HKEY_CURRENT_USER\\Software\\" + vendor.path + "\\NativeMessagingHosts\\" + browser.HostName + "]\r\n@=\"" + escaped + "\"\r\n"
			units := utf16.Encode([]rune(text))
			encoded := make([]byte, 2+len(units)*2)
			encoded[0] = 255
			encoded[1] = 254
			for i, u := range units {
				binary.LittleEndian.PutUint16(encoded[2+i*2:], u)
			}
			if err = os.WriteFile(filepath.Join(output, "register-"+vendor.name+".reg"), encoded, 0600); err != nil {
				return nil, err
			}
		}
	}
	instructions := "Prepared only; no browser registration was changed.\nKeep this directory in its final location before registration.\nWindows: inspect/import the register-<browser>.reg file for your browser.\nLinux Chromium: copy dev.heimdall.browser.json to ~/.config/chromium/NativeMessagingHosts/.\nLinux Chrome: use ~/.config/google-chrome/NativeMessagingHosts/.\nUse the correct native-host path for nondefault browser installations.\nLoad the unpacked extension, then explicitly pair its profile using heimdall browser pair PROFILE.\n"
	if err = os.WriteFile(filepath.Join(output, "SETUP.txt"), []byte(instructions), 0600); err != nil {
		return nil, err
	}
	return map[string]string{"status": "prepared_not_registered", "manifest": manifestPath, "extension_id": extensionID, "data_dir": dataDir}, nil
}
