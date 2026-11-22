package room

import "github.com/Frankhuex/sanguogao/internal/util/urandom"

const (
	maxCodeRetries = 10
	// 剔除易混淆字符 0/O/1/I/L
	roomCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
)

func (m *Manager) generateUniqueCodeLocked() (string, error) {
	for range maxCodeRetries {
		code, err := urandom.String(roomCodeAlphabet, RoomCodeLength)
		if err != nil {
			return "", err
		}
		if _, exists := m.roomIDToRoom[code]; !exists {
			return code, nil
		}
	}
	return "", ErrCodeExhausted
}
