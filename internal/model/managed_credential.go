package model

import "time"

// ManagedCredential stores a reversible, encrypted copy of a local child's
// current password. The bcrypt login hash remains the authentication source of
// truth; this record exists only for root-managed family accounts.
type ManagedCredential struct {
	UserID        string `gorm:"primaryKey;type:char(32)"`
	FormatVersion uint16 `gorm:"not null;default:1"`
	KeyID         string `gorm:"not null;type:char(64);index"`
	Ciphertext    []byte `gorm:"not null"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	User          *User `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}
