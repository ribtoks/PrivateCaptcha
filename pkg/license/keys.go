//go:build !tests

package license

import (
	"crypto/ed25519"
	_ "embed"
	"encoding/base64"
	"encoding/json"
)

//go:embed public.keys
var embeddedKeys []byte

type hardcodedKey struct {
	KeyID   int    `json:"KeyID"`
	KeyData string `json:"KeyData"`
}

func activationKeys(hardcodedKeys []*hardcodedKey) ([]*ActivationKey, error) {
	keys := make([]*ActivationKey, 0, len(hardcodedKeys))
	for _, k := range hardcodedKeys {
		publicKey, err := base64.StdEncoding.DecodeString(k.KeyData)
		if err != nil {
			return nil, err
		}

		keys = append(keys, &ActivationKey{
			ID:   k.KeyID,
			Data: ed25519.PublicKey(publicKey),
		})
	}
	return keys, nil
}

func ActivationKeys() ([]*ActivationKey, error) {
	var parsedKeys []*hardcodedKey

	if err := json.Unmarshal(embeddedKeys, &parsedKeys); err != nil {
		return nil, err
	}

	return activationKeys(parsedKeys)
}
