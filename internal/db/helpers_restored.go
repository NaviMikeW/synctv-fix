package db

import "gorm.io/gorm"

func Transactional(txFunc func(*gorm.DB) error) (err error) {
	tx := db.Begin()
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	err = txFunc(tx)

	return err
}

// HandleUpdateResult handles update errors and zero-row updates consistently.
func HandleUpdateResult(result *gorm.DB, entityName string) error {
	if result.Error != nil {
		return HandleNotFound(result.Error, entityName)
	}

	if result.RowsAffected == 0 {
		return NotFoundError(entityName)
	}

	return nil
}
