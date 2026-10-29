package handoff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"
)

// encoding/json is the codec. This guard rejects its permissive duplicate-key
// and invalid-Unicode behavior before decoding the public wire contract.
func strictJSON(raw []byte, dst any) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("invalid UTF8")
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		i++
		for ; i < len(raw) && raw[i] != '"'; i++ {
			if raw[i] != '\\' {
				continue
			}
			i++
			if i >= len(raw) {
				break
			}
			if raw[i] != 'u' {
				continue
			}
			if i+4 >= len(raw) {
				return fmt.Errorf("incomplete Unicode escape")
			}
			n, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
			if err != nil {
				return err
			}
			i += 4
			if n >= 0xdc00 && n <= 0xdfff {
				return fmt.Errorf("unpaired Unicode surrogate")
			}
			if n >= 0xd800 && n <= 0xdbff {
				if i+6 >= len(raw) || string(raw[i+1:i+3]) != "\\u" {
					return fmt.Errorf("unpaired Unicode surrogate")
				}
				m, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
				if err != nil || m < 0xdc00 || m > 0xdfff {
					return fmt.Errorf("unpaired Unicode surrogate")
				}
				i += 6
			}
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := walkJSON(dec, 0); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON value")
	}
	dec = json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	return dec.Decode(dst)
}
func walkJSON(dec *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("JSON nesting limit exceeded")
	}
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return fmt.Errorf("expected JSON object key")
			}
			if seen[name] {
				return fmt.Errorf("duplicate JSON key %q", name)
			}
			seen[name] = true
			if err := walkJSON(dec, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for dec.More() {
			if err := walkJSON(dec, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
	_, err = dec.Token()
	return err
}
