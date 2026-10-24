package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Configuration files (spec/surface/cli/index.md §7). The error texts
// reproduce the reference's JSON library for the observed cases.

var configKeys = []string{"logFile", "output", "verbose", "dataDir", "trustNewIdentities", "scrubLog", "busName", "dbus", "account", "serviceEnvironment", "disableSendLog", "dbusSystem"}

func configFiles(getenv func(string) string) []string {
	files := []string{"/etc/signal-cli/config.json"}
	switch {
	case getenv("SIGNAL_CLI_CONFIG") != "":
		files = append(files, getenv("SIGNAL_CLI_CONFIG"))
	case getenv("XDG_CONFIG_HOME") != "":
		files = append(files, filepath.Join(getenv("XDG_CONFIG_HOME"), "signal-cli", "config.json"))
	default:
		files = append(files, filepath.Join(getenv("HOME"), ".config", "signal-cli", "config.json"))
	}
	return files
}

// jsonLocation renders a byte offset as the reference does.
func jsonLocation(data []byte, offset int64) (line, col int) {
	line, col = 1, 1
	for i := int64(0); i < offset-1 && i < int64(len(data)); i++ {
		if data[i] == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	return line, col
}

func loadConfigFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, userError(fmt.Sprintf("Failed to load config from %s: %v", path, err))
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		var se *json.SyntaxError
		detail := err.Error()
		if errors.As(err, &se) {
			line, col := jsonLocation(data, se.Offset)
			ch := byte(' ')
			if se.Offset >= 1 && int(se.Offset) <= len(data) {
				ch = data[se.Offset-1]
			}
			expect := "was expecting double-quote to start field name"
			if !strings.Contains(se.Error(), "object key") {
				expect = "expected a valid value (JSON String, Number, Array, Object or token 'null', 'true' or 'false')"
			}
			detail = fmt.Sprintf("Unexpected character ('%c' (code %d)): %s\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d]", ch, ch, expect, line, col)
		}
		return nil, userError(fmt.Sprintf("Failed to load config from %s: %s", path, detail))
	}
	return m, nil
}

func normalizeEnum(s string, stripNonAlnum bool) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if stripNonAlnum {
		s = strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return r
			}
			return -1
		}, s)
	}
	return s
}

func configParseError(detail, field string) error {
	return userError("Failed to parse configuration file(s): " + detail + "\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: org.asamk.signal.GlobalConfig[\"" + field + "\"])")
}

// applyConfig loads and merges the configuration files into d.
func applyConfig(d *globalDefaults, getenv func(string) string) error {
	merged := map[string]any{}
	for _, f := range configFiles(getenv) {
		m, err := loadConfigFile(f)
		if err != nil {
			return err
		}
		for k, v := range m {
			merged[k] = v
		}
	}
	for k := range merged {
		known := false
		for _, c := range configKeys {
			known = known || c == k
		}
		if !known {
			quoted := make([]string, len(configKeys))
			for i, c := range configKeys {
				quoted[i] = `"` + c + `"`
			}
			return configParseError(fmt.Sprintf("Unrecognized field %q (class org.asamk.signal.GlobalConfig), not marked as ignorable (%d known properties: %s)",
				k, len(configKeys), strings.Join(quoted, ", ")), k)
		}
	}
	str := func(k string) (string, bool) {
		s, ok := merged[k].(string)
		return s, ok
	}
	boolean := func(k string, dst *bool) {
		if b, ok := merged[k].(bool); ok {
			*dst = b
		}
	}
	if s, ok := str("output"); ok {
		switch normalizeEnum(s, true) {
		case "plaintext":
			d.output = "plain-text"
		case "json":
			d.output = "json"
		default:
			return configParseError("Cannot construct instance of `org.asamk.signal.OutputType`, problem: Invalid output type: "+s, "output")
		}
	}
	if s, ok := str("serviceEnvironment"); ok {
		switch v := normalizeEnum(s, false); v {
		case "live", "staging":
			d.serviceEnv = v
		default:
			return configParseError("Cannot construct instance of `org.asamk.signal.ServiceEnvironmentCli`, problem: Invalid service environment: "+s, "serviceEnvironment")
		}
	}
	if s, ok := str("trustNewIdentities"); ok {
		switch normalizeEnum(s, true) {
		case "always":
			d.trustNew = "always"
		case "onfirstuse":
			d.trustNew = "on-first-use"
		case "never":
			d.trustNew = "never"
		default:
			return configParseError("Cannot construct instance of `org.asamk.signal.TrustNewIdentityCli`, problem: Invalid trust new identities: "+s, "trustNewIdentities")
		}
	}
	if n, ok := merged["verbose"].(json.Number); ok {
		if v, err := n.Int64(); err == nil {
			d.verbose = int(v)
		}
	}
	if s, ok := str("logFile"); ok {
		d.logFile = s
	}
	if s, ok := str("dataDir"); ok {
		d.dataDir, d.dataDirConfigured = s, true
	}
	if s, ok := str("busName"); ok {
		d.busName = s
	}
	boolean("scrubLog", &d.scrubLog)
	boolean("dbus", &d.dbus)
	boolean("dbusSystem", &d.dbusSystem)
	boolean("disableSendLog", &d.disableSendLog)
	return nil // "account" is accepted and ignored, as in 0.14.7
}
