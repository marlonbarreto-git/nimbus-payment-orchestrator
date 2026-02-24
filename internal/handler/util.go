package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
)

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

func randInt(max int) int {
	return mrand.Intn(max)
}
