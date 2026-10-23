package object

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/casosorg/casos/util"
)

const accessTokenPrefix = "casos_"

// AccessToken lets a program outside the browser, such as an AI coding agent
// talking to the MCP endpoint, act as the user who created it. Only a hash of
// the secret is stored; the secret itself is shown once, when it is created.
type AccessToken struct {
	Owner        string `xorm:"varchar(100) notnull pk" json:"owner"`
	Name         string `xorm:"varchar(100) notnull pk" json:"name"`
	CreatedTime  string `xorm:"varchar(100)" json:"createdTime"`
	LastUsedTime string `xorm:"varchar(100)" json:"lastUsedTime"`

	Hash     string `xorm:"varchar(100) unique" json:"-"`
	Prefix   string `xorm:"varchar(20)" json:"prefix"`
	ReadOnly bool   `xorm:"notnull default false" json:"readOnly"`
}

func GetAccessTokens(owner string) ([]*AccessToken, error) {
	tokens := []*AccessToken{}
	err := ormer.Engine.Desc("created_time").Find(&tokens, &AccessToken{Owner: owner})
	return tokens, err
}

func hashAccessToken(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// AddAccessToken creates a token and returns its secret, which cannot be
// recovered afterwards.
func AddAccessToken(owner, name string, readOnly bool) (*AccessToken, string, error) {
	name = strings.TrimSpace(name)
	if owner == "" {
		return nil, "", fmt.Errorf("owner cannot be empty")
	}
	if name == "" {
		return nil, "", fmt.Errorf("name cannot be empty")
	}
	if len(name) > 100 {
		return nil, "", fmt.Errorf("name is too long")
	}

	existed, err := ormer.Engine.Exist(&AccessToken{Owner: owner, Name: name})
	if err != nil {
		return nil, "", err
	}
	if existed {
		return nil, "", fmt.Errorf("a token named %s already exists", name)
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", err
	}
	secret := accessTokenPrefix + hex.EncodeToString(raw)

	token := &AccessToken{
		Owner:       owner,
		Name:        name,
		ReadOnly:    readOnly,
		CreatedTime: util.GetCurrentTime(),
		Hash:        hashAccessToken(secret),
		Prefix:      secret[:len(accessTokenPrefix)+6],
	}
	if _, err := ormer.Engine.Insert(token); err != nil {
		return nil, "", err
	}
	return token, secret, nil
}

func DeleteAccessToken(owner, name string) (bool, error) {
	affected, err := ormer.Engine.Delete(&AccessToken{Owner: owner, Name: name})
	return affected != 0, err
}

// VerifyAccessToken resolves a secret to the token it belongs to, or nil when
// it matches none. A built-in account that has since been forbidden or deleted
// takes its tokens down with it.
func VerifyAccessToken(secret string) (*AccessToken, error) {
	if !strings.HasPrefix(secret, accessTokenPrefix) {
		return nil, nil
	}

	token := AccessToken{Hash: hashAccessToken(secret)}
	existed, err := ormer.Engine.Get(&token)
	if err != nil || !existed {
		return nil, err
	}

	if IsSigninEnabled() {
		user, err := GetUserByRuntimeName(token.Owner)
		if err != nil {
			return nil, err
		}
		if user == nil || user.IsForbidden || user.IsDeleted {
			return nil, nil
		}
	}

	token.LastUsedTime = util.GetCurrentTime()
	_, _ = ormer.Engine.Where("owner = ? AND name = ?", token.Owner, token.Name).Cols("last_used_time").Update(&token)
	return &token, nil
}
