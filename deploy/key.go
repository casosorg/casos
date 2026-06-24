package deploy

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"

	"golang.org/x/crypto/ssh"
)

type NodeDeployKeyPair struct {
	PrivateKey string
	PublicKey  string
}

func GenerateNodeDeployKeyPair() (*NodeDeployKeyPair, error) {
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, err
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})

	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		return nil, err
	}

	return &NodeDeployKeyPair{
		PrivateKey: string(privatePEM),
		PublicKey:  strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))),
	}, nil
}
