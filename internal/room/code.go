package room

import (
	"crypto/rand"
	"math/big"
)

const (
	maxCodeRetries = 10
	// 剔除易混淆字符 0/O/1/I/L
	roomCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
)

func (m *Manager) generateUniqueCodeLocked() (string, error) {
	alphabetLen := big.NewInt(int64(len(roomCodeAlphabet)))
	buf := make([]byte, RoomCodeLength)

	for range maxCodeRetries {
		for i := range RoomCodeLength {
			n, err := rand.Int(rand.Reader, alphabetLen)
			if err != nil {
				return "", err
			}
			buf[i] = roomCodeAlphabet[n.Int64()]
		}
		code := string(buf)
		if _, exists := m.rooms[code]; !exists {
			return code, nil
		}
	}
	return "", ErrCodeExhausted
}
