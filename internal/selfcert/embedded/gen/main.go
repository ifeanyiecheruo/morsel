// Command gen regenerates the embedded local dev certificate committed at
// internal/selfcert/embedded/{cert,key}.pem.b64. Run from the repo root:
//
//	go run internal/selfcert/embedded/gen/main.go
package main

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/ifeanyiecheruo/morsel/internal/selfcert"
)

const outDir = "internal/selfcert/embedded"

func main() {
	cert, err := selfcert.GenerateSelfSignedWildcard(selfcert.LocalBaseDomain, 10*365*24*time.Hour)
	must(err)

	key, ok := cert.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		fmt.Fprintln(os.Stderr, "unexpected private key type")
		os.Exit(1)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	keyDER, err := x509.MarshalECPrivateKey(key)
	must(err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	must(writeB64(outDir+"/cert.pem.b64", certPEM))
	must(writeB64(outDir+"/key.pem.b64", keyPEM))
	fmt.Println("wrote", outDir+"/cert.pem.b64", "and", outDir+"/key.pem.b64")
}

func writeB64(path string, pemBytes []byte) error {
	return os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(pemBytes)), 0o644)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
