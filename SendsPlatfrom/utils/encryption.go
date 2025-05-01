package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"

	"github.com/sirupsen/logrus"
)

var AcquiesceKey = ""
var Encrypt *Encryption

// AES 加密算法
type Encryption struct {
	key string
}

func init() {
	Encrypt = NewEncryption()
}

func NewEncryption() *Encryption {
	return &Encryption{}
}

// 去掉填充的部分
func UnPadPwd(dst []byte) ([]byte, error) {
	//if len(dst) <= 0 {
	//	return dst, errors.New("长度有误")
	//}
	// 去掉的长度
	unpadNum := int(dst[len(dst)-1])
	strErr := "error"
	op := []byte(strErr)
	if len(dst) < unpadNum {
		return op, nil
	}
	str := dst[:(len(dst) - unpadNum)]
	return str, nil
}

func pkcs7Padding(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padtext...)
}

func encrypt(message, key string) string {
	keyBytes := []byte(key)
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		panic(err)
	}
	messageBytes := []byte(message)
	messageBytes = pkcs7Padding(messageBytes, block.BlockSize())
	ciphertext := make([]byte, len(messageBytes))
	iv := make([]byte, block.BlockSize())
	encrypter := cipher.NewCBCEncrypter(block, iv)
	encrypter.CryptBlocks(ciphertext, messageBytes)
	return base64.StdEncoding.EncodeToString(ciphertext)
}

// 填充密码长度
func PadPwd(srcByte []byte, blockSize int) []byte {
	padNum := blockSize - len(srcByte)%blockSize
	ret := bytes.Repeat([]byte{byte(padNum)}, padNum)
	srcByte = append(srcByte, ret...)
	return srcByte
}

// 加密
func (k *Encryption) AesEncoding(src string) string {
	keyBytes := []byte(k.key)
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		panic(err)
	}
	messageBytes := []byte(src)
	messageBytes = pkcs7Padding(messageBytes, block.BlockSize())
	ciphertext := make([]byte, len(messageBytes))
	iv := make([]byte, block.BlockSize())
	encrypter := cipher.NewCBCEncrypter(block, iv)
	encrypter.CryptBlocks(ciphertext, messageBytes)
	return base64.StdEncoding.EncodeToString(ciphertext)
}

// 解密
func (k *Encryption) AesDecoding(pwd string) (error, string) {
	pwdByte := []byte(pwd)
	pwdByte, err := base64.StdEncoding.DecodeString(pwd)
	if err != nil {
		return err, pwd
	}
	key := k.key
	block, errBlock := aes.NewCipher([]byte(key))
	if errBlock != nil {
		return errBlock, pwd
	}
	// 从密文中提取初始向量
	if len(pwdByte) < block.BlockSize() {
		return errors.New("密文长度有误"), pwd
	}
	iv := make([]byte, block.BlockSize())

	// 创建一个 CBC 模式的解密器
	mode := cipher.NewCBCDecrypter(block, iv)
	dst := make([]byte, len(pwdByte))
	mode.CryptBlocks(dst, pwdByte)
	dst, err = UnPadPwd(dst)
	if err != nil {
		logrus.Info("[AESRROR]:%v\n", err.Error())
		return err, ""
	}
	return nil, string(dst)
}

// set方法
func (k *Encryption) SetKey(key string) {
	k.key = key
}
