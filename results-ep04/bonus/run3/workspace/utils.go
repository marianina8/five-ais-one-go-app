package main

import (
	"crypto/rand"
	"math/big"
)

const codeChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
const codeLength = 7

var (
	codeCharsLen = big.NewInt(int64(len(codeChars)))
)

func randomCode(length int) (string, error) {
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, codeCharsLen)
		if err != nil {
			return "", err
		}
		result[i] = codeChars[n.Int64()]
	}
	return string(result), nil
}