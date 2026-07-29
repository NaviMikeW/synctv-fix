package db

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/synctv-org/synctv/internal/conf"
	"github.com/synctv-org/synctv/internal/model"
	"github.com/synctv-org/synctv/internal/password"
	"github.com/synctv-org/synctv/utils"
	"golang.org/x/crypto/bcrypt"
	// import fastjson serializer
	_ "github.com/synctv-org/synctv/utils/fastJSONSerializer"
	"gorm.io/gorm"
)

const (
	initialRootPasswordFilePrefix = "initial-root-password-"
	initialRootPasswordFileSuffix = ".txt"
)

var (
	db     *gorm.DB
	dbType conf.DatabaseType
)

func Init(d *gorm.DB, t conf.DatabaseType) error {
	db = d
	dbType = t

	err := UpgradeDatabase()
	if err != nil {
		return err
	}

	if err = migrateLegacyPlaybackControlPermissions(); err != nil {
		return err
	}

	err = initGuestUser()
	if err != nil {
		return err
	}

	return initRootUser()
}

func initRootUser() error {
	if err := cleanupStaleInitialRootPasswordFiles(); err != nil {
		log.Warnf("check initial root password files: %v", err)
	}

	var roots []model.User
	err := db.Where("role = ?", model.RoleRoot).Find(&roots).Error
	if err != nil {
		return err
	}
	if len(roots) > 0 {
		for i := range roots {
			if err = rotateLegacyRootPassword(
				&roots[i],
				conf.Conf.Security.InitialRootPassword,
			); err != nil {
				return err
			}
		}

		return nil
	}

	userID := utils.SortUUID()
	initialPassword := conf.Conf.Security.InitialRootPassword
	generated := initialPassword == ""
	var initialPasswordFile string
	if generated {
		initialPassword, err = generateInitialRootPassword()
		if err != nil {
			return fmt.Errorf("failed to generate initial root password: %w", err)
		}
		initialPasswordFile, err = writeInitialRootPasswordFile(userID, initialPassword)
		if err != nil {
			return fmt.Errorf("failed to save initial root password: %w", err)
		}
	} else if err = password.Validate(initialPassword); err != nil {
		return fmt.Errorf("invalid initial root password: %w", err)
	}

	u, err := CreateUser(
		"root",
		initialPassword,
		WithID(userID),
		WithRole(model.RoleRoot),
	)
	if err != nil {
		if generated {
			_ = RemoveInitialRootPasswordFile(userID)
		}
		return err
	}

	if generated {
		log.Warnf(
			"created root user:\nid: %s\nusername: %s\ninitial password saved to %s (mode 0600); change it after signing in",
			u.ID,
			u.Username,
			initialPasswordFile,
		)
	} else {
		log.Infof(
			"created root user:\nid: %s\nusername: %s\npassword: loaded from environment",
			u.ID,
			u.Username,
		)
	}

	return nil
}

func rotateLegacyRootPassword(user *model.User, initialPassword string) error {
	if !user.UsesLegacyDefaultRootPassword() {
		return nil
	}

	generated := initialPassword == ""
	var initialPasswordFile string
	if initialPassword == "" {
		var err error
		initialPassword, err = generateInitialRootPassword()
		if err != nil {
			return fmt.Errorf("failed to generate replacement root password: %w", err)
		}
		initialPasswordFile, err = writeInitialRootPasswordFile(user.ID, initialPassword)
		if err != nil {
			return fmt.Errorf("failed to save replacement root password: %w", err)
		}
	}

	if err := password.Validate(initialPassword); err != nil {
		return fmt.Errorf("invalid initial root password: %w", err)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(initialPassword),
		bcrypt.DefaultCost,
	)
	if err != nil {
		if generated {
			_ = RemoveInitialRootPasswordFile(user.ID)
		}
		return fmt.Errorf("failed to hash initial root password: %w", err)
	}

	if err = SetUserHashedPassword(user.ID, hashedPassword); err != nil {
		if generated {
			_ = RemoveInitialRootPasswordFile(user.ID)
		}
		return fmt.Errorf("failed to rotate legacy root password: %w", err)
	}
	user.HashedPassword = hashedPassword

	if generated {
		log.Warnf(
			"replaced the legacy root/root credential for %s; replacement password saved to %s (mode 0600); change it after signing in",
			user.Username,
			initialPasswordFile,
		)
	} else {
		if err = RemoveInitialRootPasswordFile(user.ID); err != nil {
			log.Warnf("remove stale initial root password file: %v", err)
		}
		log.Warnf(
			"replaced the legacy root/root credential for %s using INITIAL_ROOT_PASSWORD",
			user.Username,
		)
	}

	return nil
}

func initialRootPasswordFilePath(userID string) (string, error) {
	if !validInitialRootPasswordFileUserID(userID) {
		return "", errors.New("invalid user ID for initial root password file")
	}

	return utils.OptFilePath(
		initialRootPasswordFilePrefix + userID + initialRootPasswordFileSuffix,
	)
}

func writeInitialRootPasswordFile(userID, value string) (string, error) {
	path, err := initialRootPasswordFilePath(userID)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}

	file, err := os.CreateTemp(filepath.Dir(path), ".initial-root-password-*")
	if err != nil {
		return "", err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if err = file.Chmod(0o600); err != nil {
		_ = file.Close()
		return "", err
	}
	if _, err = file.WriteString(value + "\n"); err != nil {
		_ = file.Close()
		return "", err
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return "", err
	}
	if err = file.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(tempPath, path); err != nil {
		return "", err
	}

	return path, nil
}

// ResetRootPasswordToRecoveryFile replaces a root user's password with a
// generated password stored in the same protected recovery-file format used
// during first-run initialization. The server must be stopped before this
// maintenance operation is run.
func ResetRootPasswordToRecoveryFile(userID string) (string, error) {
	user, err := GetUserByID(userID)
	if err != nil {
		return "", fmt.Errorf("load user: %w", err)
	}
	if !user.IsRoot() {
		return "", errors.New("the selected user is not a root user")
	}

	path, err := initialRootPasswordFilePath(userID)
	if err != nil {
		return "", err
	}
	if _, err = os.Stat(path); err == nil {
		return "", fmt.Errorf(
			"recovery file already exists at %s; use that password first",
			path,
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("check existing recovery file: %w", err)
	}

	value, err := generateInitialRootPassword()
	if err != nil {
		return "", fmt.Errorf("generate recovery password: %w", err)
	}
	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(value),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return "", fmt.Errorf("hash recovery password: %w", err)
	}
	path, err = writeInitialRootPasswordFile(userID, value)
	if err != nil {
		return "", fmt.Errorf("write recovery password file: %w", err)
	}
	if err = SetUserHashedPassword(userID, hashedPassword); err != nil {
		_ = RemoveInitialRootPasswordFile(userID)
		return "", fmt.Errorf("replace root password: %w", err)
	}

	return path, nil
}

func RemoveInitialRootPasswordFile(userID string) error {
	path, err := initialRootPasswordFilePath(userID)
	if err != nil {
		return err
	}

	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

func cleanupStaleInitialRootPasswordFiles() error {
	pattern, err := utils.OptFilePath(
		initialRootPasswordFilePrefix + "*" + initialRootPasswordFileSuffix,
	)
	if err != nil {
		return err
	}
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	var cleanupErrors []error
	for _, path := range paths {
		userID, ok := initialRootPasswordFileUserID(path)
		if !ok {
			continue
		}

		var user model.User
		loadErr := db.Where("id = ?", userID).First(&user).Error
		if loadErr != nil && !errors.Is(loadErr, gorm.ErrRecordNotFound) {
			cleanupErrors = append(
				cleanupErrors,
				fmt.Errorf("load user for %s: %w", path, loadErr),
			)
			continue
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			cleanupErrors = append(
				cleanupErrors,
				fmt.Errorf("read %s: %w", path, readErr),
			)
			continue
		}
		if loadErr == nil && user.CheckPassword(strings.TrimSpace(string(data))) {
			continue
		}

		if removeErr := os.Remove(path); removeErr != nil &&
			!errors.Is(removeErr, os.ErrNotExist) {
			cleanupErrors = append(
				cleanupErrors,
				fmt.Errorf("remove stale %s: %w", path, removeErr),
			)
		}
	}

	return errors.Join(cleanupErrors...)
}

func initialRootPasswordFileUserID(path string) (string, bool) {
	name := filepath.Base(path)
	if !strings.HasPrefix(name, initialRootPasswordFilePrefix) ||
		!strings.HasSuffix(name, initialRootPasswordFileSuffix) {
		return "", false
	}
	userID := strings.TrimSuffix(
		strings.TrimPrefix(name, initialRootPasswordFilePrefix),
		initialRootPasswordFileSuffix,
	)

	return userID, validInitialRootPasswordFileUserID(userID)
}

func validInitialRootPasswordFileUserID(userID string) bool {
	if userID == "" {
		return false
	}
	for _, char := range userID {
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '-' &&
			char != '_' {
			return false
		}
	}

	return true
}

func RootPasswordNeedsChange(user *model.User) bool {
	if user.UsesLegacyDefaultRootPassword() {
		return true
	}

	path, err := initialRootPasswordFilePath(user.ID)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func generateInitialRootPassword() (string, error) {
	randomBytes := make([]byte, 21)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	// The fixed prefix guarantees all common character classes while the random
	// suffix contributes 168 bits of entropy. The result remains within the
	// existing 32-character user-password limit.
	return "Aa1!" + base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

const (
	GuestUsername = "guest"
	GuestUserID   = "00000000000000000000000000000001"
)

func initGuestUser() error {
	user := model.User{
		ID: GuestUserID,
	}

	err := db.First(&user).Error
	if err == nil || !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	u, err := CreateUser(
		"guest",
		utils.RandString(32),
		WithRole(model.RoleUser),
		WithID(GuestUserID),
	)
	log.Infof("init guest user:\nid: %s\nusername: %s", u.ID, u.Username)

	return err
}

func DB() *gorm.DB {
	return db
}

func Close() {
	log.Info("closing db")

	sqlDB, err := db.DB()
	if err != nil {
		log.Errorf("failed to get db: %s", err.Error())
		return
	}

	err = sqlDB.Close()
	if err != nil {
		log.Errorf("failed to close db: %s", err.Error())
		return
	}
}

func Paginate(page, pageSize int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if page <= 0 {
			page = 1
		}

		if pageSize <= 0 {
			pageSize = 10
		}

		offset := (page - 1) * pageSize
		return db.Offset(offset).Limit(pageSize)
	}
}

func OrderByAsc(column string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Order(column + " asc")
	}
}

func OrderByDesc(column string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Order(column + " desc")
	}
}

func OrderByCreatedAtAsc(db *gorm.DB) *gorm.DB {
	return db.Order("created_at asc")
}

func OrderByUsersCreatedAtAsc(db *gorm.DB) *gorm.DB {
	return db.Order("users.created_at asc")
}

func OrderByCreatedAtDesc(db *gorm.DB) *gorm.DB {
	return db.Order("created_at desc")
}

func OrderByUsersCreatedAtDesc(db *gorm.DB) *gorm.DB {
	return db.Order("users.created_at desc")
}

func OrderByRoomCreatedAtAsc(db *gorm.DB) *gorm.DB {
	return db.Order("rooms.created_at asc")
}

func OrderByRoomCreatedAtDesc(db *gorm.DB) *gorm.DB {
	return db.Order("rooms.created_at desc")
}

func OrderByIDAsc(db *gorm.DB) *gorm.DB {
	return db.Order("id asc")
}

func OrderByIDDesc(db *gorm.DB) *gorm.DB {
	return db.Order("id desc")
}

func WithUser(db *gorm.DB) *gorm.DB {
	return db.Preload("User")
}

func WhereRoomID(roomID string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("room_id = ?", roomID)
	}
}

func PreloadRoomMembers(scopes ...func(*gorm.DB) *gorm.DB) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Preload("RoomMembers", func(db *gorm.DB) *gorm.DB {
			return db.Scopes(scopes...)
		})
	}
}

func PreloadUserProviders(scopes ...func(*gorm.DB) *gorm.DB) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Preload("UserProviders", func(db *gorm.DB) *gorm.DB {
			return db.Scopes(scopes...)
		})
	}
}

func WhereUserID(userID string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("user_id = ?", userID)
	}
}

func WhereCreatorID(creatorID string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("creator_id = ?", creatorID)
	}
}

// column cannot be a user parameter
func WhereEqual(column string, value any) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("? = ?", column, value)
	}
}

// column cannot be a user parameter
func WhereLike(column, value string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		switch dbType {
		case conf.DatabaseTypePostgres:
			return db.Where("? ILIKE ?", column, utils.LIKE(value))
		default:
			return db.Where("? LIKE ?", column, utils.LIKE(value))
		}
	}
}

func WhereMovieNameLikeOrURLLike(name, url string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		switch dbType {
		case conf.DatabaseTypePostgres:
			return db.Where(
				"base_name ILIKE ? OR base_url ILIKE ?",
				utils.LIKE(name),
				utils.LIKE(url),
			)
		default:
			return db.Where(
				"base_name LIKE ? OR base_url LIKE ?",
				utils.LIKE(name),
				utils.LIKE(url),
			)
		}
	}
}

func WhereRoomNameLikeOrCreatorInOrIDLike(
	name string,
	ids []string,
	id string,
) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		switch dbType {
		case conf.DatabaseTypePostgres:
			return db.Where(
				"name ILIKE ? OR creator_id IN ? OR id ILIKE ?",
				utils.LIKE(name),
				ids,
				id,
			)
		default:
			return db.Where(
				"name LIKE ? OR creator_id IN ? OR id LIKE ?",
				utils.LIKE(name),
				ids,
				id,
			)
		}
	}
}

func WhereRoomNameLikeOrCreatorInOrRoomsIDLike(
	name string,
	ids []string,
	id string,
) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		switch dbType {
		case conf.DatabaseTypePostgres:
			return db.Where(
				"name ILIKE ? OR creator_id IN ? OR rooms.id ILIKE ?",
				utils.LIKE(name),
				ids,
				id,
			)
		default:
			return db.Where(
				"name LIKE ? OR creator_id IN ? OR rooms.id LIKE ?",
				utils.LIKE(name),
				ids,
				id,
			)
		}
	}
}

func WhereRoomNameLikeOrIDLike(name, id string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		switch dbType {
		case conf.DatabaseTypePostgres:
			return db.Where("name ILIKE ? OR id ILIKE ?", utils.LIKE(name), id)
		default:
			return db.Where("name LIKE ? OR id LIKE ?", utils.LIKE(name), id)
		}
	}
}

func WhereRoomNameLike(name string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		switch dbType {
		case conf.DatabaseTypePostgres:
			return db.Where("name ILIKE ?", utils.LIKE(name))
		default:
			return db.Where("name LIKE ?", utils.LIKE(name))
		}
	}
}

func WhereUsernameLike(name string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		switch dbType {
		case conf.DatabaseTypePostgres:
			return db.Where("username ILIKE ?", utils.LIKE(name))
		default:
			return db.Where("username LIKE ?", utils.LIKE(name))
		}
	}
}

func WhereCreatorIDIn(ids []string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("creator_id IN ?", ids)
	}
}

func Select(columns ...string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Select(columns)
	}
}

func WhereStatus(status model.RoomStatus) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("status = ?", status)
	}
}

func WhereRole(role model.Role) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("role = ?", role)
	}
}

func WhereUsernameLikeOrIDIn(name string, ids []string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		switch dbType {
		case conf.DatabaseTypePostgres:
			return db.Where("username ILIKE ? OR id IN ?", utils.LIKE(name), ids)
		default:
			return db.Where("username LIKE ? OR id IN ?", utils.LIKE(name), ids)
		}
	}
}

func WhereIDIn(ids []string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("id IN ?", ids)
	}
}

func WhereRoomSettingWithoutHidden() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("hidden = ?", false)
	}
}

func WhereIDLike(id string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		switch dbType {
		case conf.DatabaseTypePostgres:
			return db.Where("id ILIKE ?", utils.LIKE(id))
		default:
			return db.Where("id LIKE ?", utils.LIKE(id))
		}
	}
}

func WhereRoomsIDLike(id string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		switch dbType {
		case conf.DatabaseTypePostgres:
			return db.Where("rooms.id ILIKE ?", utils.LIKE(id))
		default:
			return db.Where("rooms.id LIKE ?", utils.LIKE(id))
		}
	}
}

func WhereRoomMemberStatus(status model.RoomMemberStatus) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("room_members.status = ?", status)
	}
}

func WhereRoomMemberRole(role model.RoomMemberRole) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("room_members.role = ?", role)
	}
}

type NotFoundError string

func (e NotFoundError) Error() string {
	return string(e) + " not found"
}

func HandleNotFound(err error, errMsg ...string) error {
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return NotFoundError(strings.Join(errMsg, " "))
	}
	return err
}
