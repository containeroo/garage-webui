package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
)

// JobOwner computes a keyed fingerprint so job visibility follows the same Garage credential across browser sessions.
func (s *Service) JobOwner(c Credential) ([]byte, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, s.encryptionKey)
	mac.Write([]byte("garage-webui-job-owner-v1"))
	mac.Write(data)
	return mac.Sum(nil), nil
}

// EncryptJobCredential stores an independent encrypted job credential bound to its immutable job ID.
func (s *Service) EncryptJobCredential(id string, c Credential) ([]byte, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, s.encryption.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.encryption.Seal(nonce, nonce, data, []byte("job:"+id)), nil
}

// DecryptJobCredential authenticates the database payload before creating a Garage client.
func (s *Service) DecryptJobCredential(id string, encrypted []byte) (Credential, error) {
	var c Credential
	if len(encrypted) < s.encryption.NonceSize() {
		return c, errors.New("invalid encrypted job credential")
	}
	data, err := s.encryption.Open(nil, encrypted[:s.encryption.NonceSize()], encrypted[s.encryption.NonceSize():], []byte("job:"+id))
	if err != nil {
		return c, errors.New("invalid encrypted job credential")
	}
	if err = json.Unmarshal(data, &c); err != nil {
		return Credential{}, errors.New("invalid encrypted job credential")
	}
	return c, nil
}
