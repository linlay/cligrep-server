package data

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"hash"
	"strconv"
	"strings"
)

const (
	passwordHashScheme     = "pbkdf2_sha256"
	passwordHashIterations = 120000
	passwordHashSaltLength = 16
	passwordHashKeyLength  = 32
)

func hashPassword(password string) (string, error) {
	salt := make([]byte, passwordHashSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	key := pbkdf2Key([]byte(password), salt, passwordHashIterations, passwordHashKeyLength, sha256.New)
	return fmt.Sprintf(
		"%s$%d$%s$%s",
		passwordHashScheme,
		passwordHashIterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func verifyPasswordHash(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordHashScheme {
		return false, fmt.Errorf("unsupported password hash format")
	}

	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false, fmt.Errorf("invalid password hash iterations")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false, fmt.Errorf("decode password hash salt: %w", err)
	}
	expectedKey, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, fmt.Errorf("decode password hash key: %w", err)
	}

	derivedKey := pbkdf2Key([]byte(password), salt, iterations, len(expectedKey), sha256.New)
	return subtle.ConstantTimeCompare(expectedKey, derivedKey) == 1, nil
}

func pbkdf2Key(password, salt []byte, iter, keyLen int, newHash func() hash.Hash) []byte {
	hashLen := newHash().Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen
	derived := make([]byte, 0, numBlocks*hashLen)

	var blockBuf [4]byte
	for block := 1; block <= numBlocks; block++ {
		blockBuf[0] = byte(block >> 24)
		blockBuf[1] = byte(block >> 16)
		blockBuf[2] = byte(block >> 8)
		blockBuf[3] = byte(block)

		prf := hmac.New(newHash, password)
		prf.Write(salt)
		prf.Write(blockBuf[:])
		sum := prf.Sum(nil)

		u := make([]byte, len(sum))
		copy(u, sum)
		t := make([]byte, len(sum))
		copy(t, sum)

		for round := 1; round < iter; round++ {
			prf = hmac.New(newHash, password)
			prf.Write(u)
			u = prf.Sum(nil)
			for i := range t {
				t[i] ^= u[i]
			}
		}
		derived = append(derived, t...)
	}

	return derived[:keyLen]
}
