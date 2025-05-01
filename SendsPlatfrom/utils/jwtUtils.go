package utils

import (
	"crypto/md5"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v4"
	"time"
)

// 过期时间
var expirationTime = time.Now().Add(25 * time.Hour).Unix()

type Claims struct {
	OpenId string `json:"openId"`
	StuNum string `json:"stuNum"`
	Exp    int64  `json:"exp"`
	jwt.StandardClaims
}

type MassesClaims struct {
	OpenId   string `json:"openId"`
	NickName string `json:"nick_name"`
	Exp      int64  `json:"exp"`
	jwt.StandardClaims
}

type AdminClaims struct {
	OpenId       string `json:"openId"`
	Organization uint32 `json:"organization"`
	Exp          int64  `json:"exp"`
	jwt.StandardClaims
}

// GetMd5
// 生成 md5
func GetMd5(s string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(s)))
}

var userKey = []byte("")
var adminKey = []byte("")

func GenerateUserToken(openid string, stunum string) string {
	UserClaim := &Claims{
		OpenId:         openid,
		StuNum:         stunum,
		Exp:            expirationTime,
		StandardClaims: jwt.StandardClaims{},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, UserClaim)
	tokenString, err := token.SignedString(userKey)
	if err != nil {
		return ""
	}
	return tokenString
}

func GenerateMassesToken(openid string, nickName string) string {
	MassesClaim := &MassesClaims{
		OpenId:         openid,
		Exp:            expirationTime,
		NickName:       nickName,
		StandardClaims: jwt.StandardClaims{},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, MassesClaim)
	tokenString, err := token.SignedString(userKey)
	if err != nil {
		return ""
	}
	return tokenString
}

func GenerateAdminToken(openid string, organization uint32) string {
	AdminClaim := &AdminClaims{
		OpenId:         openid,
		Organization:   organization,
		StandardClaims: jwt.StandardClaims{},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, AdminClaim)
	tokenString, err := token.SignedString(adminKey)
	if err != nil {
		return ""
	}
	return tokenString
}

// AnalyseToken
// 解析 token
func AnalyseUserToken(tokenString string) (*Claims, error) {
	userClaim := new(Claims)
	claims, err := jwt.ParseWithClaims(tokenString, userClaim, func(token *jwt.Token) (interface{}, error) {
		return userKey, nil
	})
	if err != nil {
		return nil, err
	}
	if !claims.Valid {
		return nil, fmt.Errorf("analyse Token Error:%v", err)
	}
	fmt.Println(userClaim.Exp)
	//if userClaim.Exp < time.Now().Unix() {
	//	return userClaim, errors.New("Token已过期")
	//}
	return userClaim, nil
}

func AnalyseMassesToken(tokenString string) (*MassesClaims, error) {
	massesClaim := new(MassesClaims)
	claims, err := jwt.ParseWithClaims(tokenString, massesClaim, func(token *jwt.Token) (interface{}, error) {
		return userKey, nil
	})
	if err != nil {
		return nil, err
	}
	if !claims.Valid {
		return nil, fmt.Errorf("analyse Token Error:%v", err)
	}
	return massesClaim, nil
}

func AnalyseAdminToken(tokenString string) (*AdminClaims, error) {
	adminClaim := new(AdminClaims)
	claims, err := jwt.ParseWithClaims(tokenString, adminClaim, func(token *jwt.Token) (interface{}, error) {
		return adminKey, nil
	})
	if err != nil {
		return nil, err
	}
	if !claims.Valid {
		return nil, fmt.Errorf("analyse Token Error:%v", err)
	}
	return adminClaim, nil
}

// 检查token是否过期
func ParseToken(token string) (err error) {
	claims, err := AnalyseUserToken(token)
	if err != nil {
		return
	}
	if claims.Exp < time.Now().Unix() {
		return errors.New("Token已过期")
	}
	return
}

func parseToken(exp int64) (err error) {
	if exp < time.Now().Unix() {
		return errors.New("Token已过期")
	}
	return
}
