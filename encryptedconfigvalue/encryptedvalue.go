// Copyright 2017 Palantir Technologies. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package encryptedconfigvalue

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// EncryptedValue represents a value that has been encrypted using encrypted-config-value. The value can be decrypted
// when provided with a key (the type of the key should be a decryption key that can decrypt the value) and supports
// returning a string representation that can be used to serialize the EncryptedValue.
type EncryptedValue interface {
	// Decrypt decrypts this value using the provided key and returns the decrypted string. The provided key must be
	// a decryption key that supports decrypting the stored encrypted value and is compatible with the encryption
	// algorithm. Returns an error if the provided decryption key is not a valid key for decrypting this value or if
	// an error is encountered during decryption.
	Decrypt(key KeyWithType) (string, error)

	// ToSerializable returns the string that can be used to serialize this EncryptedValue. The returned string can
	// be used as input to the "NewEncryptedValue" function to recreate the value. The serialized string is of the
	// form "enc:<base64-encoded-content>". The exact content that is base64-encoded is dependent on the concrete
	// implementation.
	ToSerializable() (string, error)
}

const encPrefix = "enc:"

// MustNewEncryptedValue returns the result of calling NewEncryptedValue with the provided arguments. Panics if the call
// returns an error. This function should only be used when instantiating values that are known to be formatted
// correctly.
func MustNewEncryptedValue(evStr string) EncryptedValue {
	ev, err := NewEncryptedValue(evStr)
	if err != nil {
		panic(err)
	}
	return ev
}

// NewEncryptedValue creates a new encrypted value from its string representation. The string representation of an
// EncryptedValue is of the form "enc:<base64-text>".
//
// EncryptedValue has a legacy format (values generated by implementations up to version 1.0.2) and a new format
// (values generated by implementations after 1.0.2). In the legacy format, the <base64-text> encodes the bytes of the
// ciphertext. In the new format, the <base64-text> encodes the JSON string representation of the EncryptedValue.
//
// If the decoded <base64-text> is valid JSON, this function treats it as a new format value; otherwise, it decodes it
// as a legacy format value.
func NewEncryptedValue(evStr string) (EncryptedValue, error) {
	if !strings.HasPrefix(evStr, encPrefix) {
		return nil, fmt.Errorf(`encrypted value must be of the form "%s...", was: %q`, encPrefix, evStr)
	}

	contentB64 := evStr[len(encPrefix):]
	evContentBytes, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode content: %v", err)
	}

	emptyJSON := struct{}{}
	if err := json.Unmarshal(evContentBytes, &emptyJSON); err != nil {
		// value is not JSON: assume it is legacy encrypted-value
		return &legacyEncryptedValue{
			encryptedBytes: evContentBytes,
		}, nil
	}

	var evWrapper encryptedValWrapper
	if err := json.Unmarshal(evContentBytes, &evWrapper); err != nil {
		return nil, err
	}
	return evWrapper.val, nil
}

func encryptedValToSerializable(ev EncryptedValue) (string, error) {
	jsonBytes, err := json.Marshal(ev)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(encPrefix + base64.StdEncoding.EncodeToString(jsonBytes)), nil
}

type encryptedValWrapper struct {
	val EncryptedValue
}

func (ev *encryptedValWrapper) UnmarshalJSON(data []byte) error {
	val := struct {
		Algorithm AlgorithmType `json:"type"`
	}{}
	if err := json.Unmarshal(data, &val); err != nil {
		return err
	}
	var evWrapper encryptedValWrapper
	switch val.Algorithm {
	default:
		return fmt.Errorf("unrecognized algorithm type: %s", val.Algorithm)
	case AES:
		var aesVal aesGCMEncryptedValue
		if err := json.Unmarshal(data, &aesVal); err != nil {
			return err
		}
		evWrapper.val = &aesVal
	case RSA:
		var rsaVal rsaOAEPEncryptedValue
		if err := json.Unmarshal(data, &rsaVal); err != nil {
			return err
		}
		evWrapper.val = &rsaVal
	}
	*ev = evWrapper
	return nil
}
