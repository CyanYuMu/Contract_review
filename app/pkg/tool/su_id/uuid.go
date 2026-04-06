package su_id

import (
	"github.com/google/uuid"
	"github.com/rs/xid"
	uuidOld "github.com/satori/go.uuid"
)

// GetWithUUID
// @description
// Version 1,基于 timestamp 和 MAC address (RFC 4122)
// Version 2,基于 timestamp, MAC address 和 POSIX UID/GID (DCE 1.1)
// Version 3, 基于 MD5 hashing (RFC 4122)
// Version 4, 基于 random numbers (RFC 4122)
// Version 5, 基于 SHA-1 hashing (RFC 4122)
func GetWithUUID() string {
	return uuidOld.NewV4().String()
}

// GetWithUUIDV7
// @description 基于时间戳和随机数的 UUID v7 (RFC 9562)
// UUID v7 结合了时间戳和随机数，提供更好的排序性和唯一性
func GetWithUUIDV7() string {
	uid, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}

	return uid.String()
}

func GetWithXid() string {
	return xid.New().String()
}
