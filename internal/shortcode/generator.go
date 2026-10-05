package shortcode

import (
	"crypto/rand"
	"math/big"
)

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
const length = 7

type Generator interface {
	GenerateCode() (string, error)
}

type Base62Generator struct{}

func (b Base62Generator) GenerateCode() (string, error) {
	result := make([]byte, length)
	max := big.NewInt(int64(len(base62)))

	for i := range result {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}

		result[i] = base62[n.Int64()]
	}

	return string(result), nil
}
