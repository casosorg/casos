package object

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	LocalUserOwner          = "casos-local"
	LocalAdminName          = "admin"
	minLocalPasswordRunes   = 8
	maxLocalPasswordBytes   = 72
	unknownUserPasswordHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
)

var ErrLocalAdminAlreadyInitialized = errors.New("local administrator is already initialized")

type LocalUser struct {
	Owner       string `xorm:"varchar(100) notnull pk" json:"owner"`
	Name        string `xorm:"varchar(100) notnull pk" json:"name"`
	CreatedTime string `xorm:"varchar(100)" json:"createdTime"`
	UpdatedTime string `xorm:"varchar(100)" json:"updatedTime"`

	DisplayName  string `xorm:"varchar(100)" json:"displayName"`
	PasswordHash string `xorm:"varchar(150)" json:"-"`
	IsAdmin      bool   `json:"isAdmin"`
	IsForbidden  bool   `json:"isForbidden"`
	IsDeleted    bool   `json:"isDeleted"`
}

func ValidateLocalPassword(password string) error {
	if !utf8.ValidString(password) {
		return errors.New("password must be valid UTF-8")
	}
	if strings.TrimSpace(password) == "" {
		return errors.New("password cannot be empty")
	}
	if utf8.RuneCountInString(password) < minLocalPasswordRunes {
		return fmt.Errorf("password must contain at least %d characters", minLocalPasswordRunes)
	}
	if len([]byte(password)) > maxLocalPasswordBytes {
		return fmt.Errorf("password cannot be longer than %d bytes", maxLocalPasswordBytes)
	}
	return nil
}

func hashLocalPassword(password string) (string, error) {
	if err := ValidateLocalPassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func GetLocalUser(name string) (*LocalUser, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("username cannot be empty")
	}

	user := &LocalUser{Owner: LocalUserOwner, Name: name}
	existed, err := ormer.Engine.Get(user)
	if err != nil {
		return nil, err
	}
	if !existed {
		return nil, nil
	}
	return user, nil
}

func IsLocalAdminInitialized() (bool, error) {
	user, err := GetLocalUser(LocalAdminName)
	return user != nil, err
}

func CreateLocalAdmin(password string) (*LocalUser, error) {
	initialized, err := IsLocalAdminInitialized()
	if err != nil {
		return nil, err
	}
	if initialized {
		return nil, ErrLocalAdminAlreadyInitialized
	}

	passwordHash, err := hashLocalPassword(password)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	user := &LocalUser{
		Owner:        LocalUserOwner,
		Name:         LocalAdminName,
		CreatedTime:  now,
		UpdatedTime:  now,
		DisplayName:  "Administrator",
		PasswordHash: passwordHash,
		IsAdmin:      true,
	}
	if _, err = ormer.Engine.Insert(user); err != nil {
		// The fixed primary key makes concurrent setup requests converge on one
		// administrator. Report the loser as an already initialized instance.
		if existing, getErr := GetLocalUser(LocalAdminName); getErr == nil && existing != nil {
			return nil, ErrLocalAdminAlreadyInitialized
		}
		return nil, err
	}
	return user, nil
}

func VerifyLocalUser(username, password string) (*LocalUser, bool, error) {
	username = strings.TrimSpace(username)
	if username != LocalAdminName {
		compareUnknownLocalPassword(password)
		return nil, false, nil
	}

	user, err := GetLocalUser(username)
	if err != nil {
		compareUnknownLocalPassword(password)
		return nil, false, err
	}
	if user == nil || user.IsDeleted || user.IsForbidden || !user.IsAdmin || user.PasswordHash == "" {
		compareUnknownLocalPassword(password)
		return nil, false, nil
	}
	if len([]byte(password)) > maxLocalPasswordBytes || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, false, nil
	}
	return user, true, nil
}

func CheckLocalUserPassword(user *LocalUser, password string) bool {
	if user == nil || len([]byte(password)) > maxLocalPasswordBytes {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil
}

func UpdateLocalUserPassword(user *LocalUser, password string) error {
	if user == nil {
		return errors.New("local user does not exist")
	}
	passwordHash, err := hashLocalPassword(password)
	if err != nil {
		return err
	}

	user.PasswordHash = passwordHash
	user.UpdatedTime = time.Now().UTC().Format(time.RFC3339Nano)
	affected, err := ormer.Engine.Where("owner = ? AND name = ?", user.Owner, user.Name).
		Cols("password_hash", "updated_time").Update(user)
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("local user does not exist")
	}
	return nil
}

func compareUnknownLocalPassword(password string) {
	if len([]byte(password)) <= maxLocalPasswordBytes {
		_ = bcrypt.CompareHashAndPassword([]byte(unknownUserPasswordHash), []byte(password))
	}
}
