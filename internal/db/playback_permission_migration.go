package db

import (
	log "github.com/sirupsen/logrus"
	"github.com/synctv-org/synctv/internal/model"
	"gorm.io/gorm"
)

// migrateLegacyPlaybackControlPermissions upgrades only the exact legacy
// default permission set. Manually customized member permissions are left
// untouched.
func migrateLegacyPlaybackControlPermissions() error {
	legacyPermissions := model.DefaultPermissions
	newPermissions := legacyPermissions | model.PermissionSetCurrentStatus

	return db.Transaction(func(tx *gorm.DB) error {
		roomSettingsResult := tx.Model(&model.RoomSettings{}).
			Where("user_default_permissions = ?", legacyPermissions).
			Update("user_default_permissions", newPermissions)
		if roomSettingsResult.Error != nil {
			return roomSettingsResult.Error
		}

		memberResult := tx.Model(&model.RoomMember{}).
			Where("role = ?", model.RoomMemberRoleMember).
			Where("user_id <> ?", GuestUserID).
			Where("permissions = ?", legacyPermissions).
			Update("permissions", newPermissions)
		if memberResult.Error != nil {
			return memberResult.Error
		}

		if roomSettingsResult.RowsAffected > 0 || memberResult.RowsAffected > 0 {
			log.WithFields(log.Fields{
				"room_settings": roomSettingsResult.RowsAffected,
				"room_members":  memberResult.RowsAffected,
			}).Info("migrated legacy playback-control permissions")
		}
		return nil
	})
}
