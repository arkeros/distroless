// Package keyring decodes ASCII-armored OpenPGP public keyrings that may
// concatenate multiple BEGIN/END PGP PUBLIC KEY BLOCK sections into one
// file.
//
// openpgp.ReadArmoredKeyRing consumes only the first armor block, so a
// vendor keyring that ships rotated/legacy keys alongside the current
// one would silently lose all but the first. ReadMultiBlock walks every
// block, accumulates into one EntityList, and skips blocks the parser
// can't decode rather than failing the whole load — vendor keyrings
// (e.g. Hummingbird's hummingbird-release.pgp) bundle keys from
// multiple eras and ProtonMail's parser rejects some legacy packet
// shapes (`first packet was not a public/private key` on 2009-era
// Red Hat key 2). Downstream signature verification fails cleanly if
// none of the parseable keys matches the signature being checked.
// An all-blocks-failed file returns the error from the first block so
// a real parsing regression surfaces instead of being masked.
package keyring

import (
	"bytes"
	"fmt"
	"os"

	"github.com/ProtonMail/go-crypto/openpgp"
)

const (
	beginMarker = "-----BEGIN PGP PUBLIC KEY BLOCK-----"
	endMarker   = "-----END PGP PUBLIC KEY BLOCK-----"
)

// ReadMultiBlock reads the file at path and returns every public key it
// contains as a flat EntityList.
func ReadMultiBlock(path string) (openpgp.EntityList, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var all openpgp.EntityList
	var firstErr error
	rest := data
	for {
		bIdx := bytes.Index(rest, []byte(beginMarker))
		if bIdx < 0 {
			break
		}
		rest = rest[bIdx:]
		eIdx := bytes.Index(rest, []byte(endMarker))
		if eIdx < 0 {
			return nil, fmt.Errorf("unterminated armor block in %q", path)
		}
		chunk := rest[:eIdx+len(endMarker)]
		rest = rest[eIdx+len(endMarker):]
		entities, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(chunk))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		all = append(all, entities...)
	}
	if len(all) == 0 {
		if firstErr != nil {
			return nil, fmt.Errorf("no usable PGP public keys in %q: %w", path, firstErr)
		}
		return nil, fmt.Errorf("no PGP public key blocks found in %q", path)
	}
	return all, nil
}
