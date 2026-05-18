package keyring

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

// armoredPublicKey serializes entity's public key as an ASCII-armored
// PGP PUBLIC KEY BLOCK — the on-disk shape every vendor keyring uses.
func armoredPublicKey(t *testing.T, entity *openpgp.Entity) string {
	t.Helper()
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	if err != nil {
		t.Fatalf("armor.Encode: %v", err)
	}
	if err := entity.Serialize(w); err != nil {
		t.Fatalf("entity.Serialize: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("armor.Close: %v", err)
	}
	return buf.String()
}

func writeKeyring(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "keyring.pgp")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestReadMultiBlock_TwoBlocks asserts the headline contract: a file
// concatenating two armor blocks yields two entities. Regression catch
// for the readArmoredKeyRing single-block bug the parser exists to
// work around.
func TestReadMultiBlock_TwoBlocks(t *testing.T) {
	a, err := openpgp.NewEntity("Test A", "", "a@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := openpgp.NewEntity("Test B", "", "b@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	path := writeKeyring(t, armoredPublicKey(t, a)+"\n"+armoredPublicKey(t, b))
	keys, err := ReadMultiBlock(path)
	if err != nil {
		t.Fatalf("ReadMultiBlock: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("len(keys) = %d, want 2", len(keys))
	}
}

// TestReadMultiBlock_BrokenBlockSkipped asserts vendor-keyring tolerance:
// one malformed armor block (a 2009-era legacy key, in the wild case) must
// not torpedo the load of a well-formed companion. Returning only the
// parseable entities lets downstream verification fail cleanly when no
// key matches, instead of failing the whole pipeline on an unrelated
// legacy artifact.
func TestReadMultiBlock_BrokenBlockSkipped(t *testing.T) {
	good, err := openpgp.NewEntity("Good", "", "good@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	broken := beginMarker + "\n\nbXktZ2FyYmFnZS1ub3QtYS1rZXk=\n" + endMarker + "\n"
	path := writeKeyring(t, broken+armoredPublicKey(t, good))

	keys, err := ReadMultiBlock(path)
	if err != nil {
		t.Fatalf("ReadMultiBlock: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("len(keys) = %d, want 1 (only the good key)", len(keys))
	}
}

// TestReadMultiBlock_AllBroken asserts the "real parsing regression"
// path: if every block fails, we surface the first error rather than
// silently returning an empty keyring (which would then accept no
// signatures and look like a key-mismatch).
func TestReadMultiBlock_AllBroken(t *testing.T) {
	broken := beginMarker + "\n\nbm90LWFyZWFsLWtleQ==\n" + endMarker + "\n"
	path := writeKeyring(t, broken+broken)
	if _, err := ReadMultiBlock(path); err == nil {
		t.Fatal("expected error when every block is malformed, got nil")
	}
}

// TestReadMultiBlock_NoBlocks asserts the no-armor-found path: a file
// with zero PGP PUBLIC KEY BLOCK markers errors loudly rather than
// returning an empty keyring.
func TestReadMultiBlock_NoBlocks(t *testing.T) {
	path := writeKeyring(t, "not a keyring at all\njust some text\n")
	if _, err := ReadMultiBlock(path); err == nil {
		t.Fatal("expected error when no PGP blocks present, got nil")
	}
}

// TestReadMultiBlock_UnterminatedBlock asserts the truncated-armor case
// fails loudly. A malformed begin-without-end almost always means a
// truncated download or a copy-paste mistake; silently slurping the
// rest of the file would be worse than failing.
func TestReadMultiBlock_UnterminatedBlock(t *testing.T) {
	path := writeKeyring(t, beginMarker+"\nbm9wZQ==\n")
	if _, err := ReadMultiBlock(path); err == nil {
		t.Fatal("expected error for unterminated armor block, got nil")
	}
}
