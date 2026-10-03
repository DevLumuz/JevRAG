package jev

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Ordered is a JSON object that keeps its keys in the order given. Go maps
// marshal with keys sorted alphabetically, which silently reorders a
// choice's options (and a noul's true/false criteria) before JEV sees them —
// and JEV leans toward the first option. Use Ordered wherever order matters.
type Ordered []KV

// KV is one key/value pair of an Ordered object.
type KV struct {
	Key   string
	Value any
}

// MarshalJSON writes the object with keys in slice order.
func (o Ordered) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, kv := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(kv.Key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(kv.Value)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// Get returns the value for key.
func (o Ordered) Get(key string) (any, bool) {
	for _, kv := range o {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return nil, false
}

// DecodeOrdered parses JSON like json.Unmarshal into `any`, except that
// every object becomes an Ordered that preserves the source key order.
func DecodeOrdered(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("jev: trailing data after JSON value")
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			var o Ordered
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := kt.(string)
				if !ok {
					return nil, fmt.Errorf("jev: object key is not a string")
				}
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				o = append(o, KV{Key: key, Value: v})
			}
			if _, err := dec.Token(); err != nil { // '}'
				return nil, err
			}
			if o == nil {
				o = Ordered{}
			}
			return o, nil
		case '[':
			arr := []any{}
			for dec.More() {
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			if _, err := dec.Token(); err != nil { // ']'
				return nil, err
			}
			return arr, nil
		}
		return nil, fmt.Errorf("jev: unexpected delimiter %v", t)
	default:
		return t, nil // string, json.Number, bool, nil
	}
}
