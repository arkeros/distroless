package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalELF builds the smallest ELF64 debug/elf will read PT_INTERP,
// DT_NEEDED and DT_RUNPATH from: a header, a program header per segment,
// and the four sections DynString needs — null, .dynstr, .dynamic and
// .shstrtab. Nothing in it runs; everything in it loads.
func minimalELF(machine elf.Machine, interp string, needed []string, runpath string) []byte {
	le := binary.LittleEndian

	// String tables.
	var dynstr bytes.Buffer
	dynstr.WriteByte(0)
	offsets := map[string]uint64{}
	add := func(s string) uint64 {
		if o, ok := offsets[s]; ok {
			return o
		}
		o := uint64(dynstr.Len())
		offsets[s] = o
		dynstr.WriteString(s)
		dynstr.WriteByte(0)
		return o
	}
	var neededOff []uint64
	for _, n := range needed {
		neededOff = append(neededOff, add(n))
	}
	var runpathOff uint64
	if runpath != "" {
		runpathOff = add(runpath)
	}
	shstrtab := []byte("\x00.dynstr\x00.dynamic\x00.shstrtab\x00")

	// .dynamic entries.
	var dynamic bytes.Buffer
	entry := func(tag elf.DynTag, val uint64) {
		binary.Write(&dynamic, le, uint64(tag))
		binary.Write(&dynamic, le, val)
	}
	for _, o := range neededOff {
		entry(elf.DT_NEEDED, o)
	}
	if runpath != "" {
		entry(elf.DT_RUNPATH, runpathOff)
	}
	entry(elf.DT_NULL, 0)

	// Layout: header, program headers, interp, dynstr, dynamic, shstrtab, section headers.
	phnum := 1
	if interp != "" {
		phnum = 2
	}
	interpBytes := []byte(interp + "\x00")
	off := uint64(64 + 56*phnum)
	interpOff := off
	off += uint64(len(interpBytes))
	dynstrOff := off
	off += uint64(dynstr.Len())
	dynamicOff := off
	off += uint64(dynamic.Len())
	shstrOff := off
	off += uint64(len(shstrtab))
	shoff := off

	var b bytes.Buffer
	// ELF header.
	b.Write([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	binary.Write(&b, le, uint16(elf.ET_DYN))
	binary.Write(&b, le, uint16(machine))
	binary.Write(&b, le, uint32(1))
	binary.Write(&b, le, uint64(0))     // entry
	binary.Write(&b, le, uint64(64))    // phoff
	binary.Write(&b, le, shoff)         // shoff
	binary.Write(&b, le, uint32(0))     // flags
	binary.Write(&b, le, uint16(64))    // ehsize
	binary.Write(&b, le, uint16(56))    // phentsize
	binary.Write(&b, le, uint16(phnum)) // phnum
	binary.Write(&b, le, uint16(64))    // shentsize
	binary.Write(&b, le, uint16(4))     // shnum
	binary.Write(&b, le, uint16(3))     // shstrndx
	// Program headers.
	phdr := func(typ elf.ProgType, offset, size uint64) {
		binary.Write(&b, le, uint32(typ))
		binary.Write(&b, le, uint32(elf.PF_R))
		binary.Write(&b, le, offset) // offset
		binary.Write(&b, le, offset) // vaddr
		binary.Write(&b, le, offset) // paddr
		binary.Write(&b, le, size)   // filesz
		binary.Write(&b, le, size)   // memsz
		binary.Write(&b, le, uint64(1))
	}
	if interp != "" {
		phdr(elf.PT_INTERP, interpOff, uint64(len(interpBytes)))
	}
	phdr(elf.PT_DYNAMIC, dynamicOff, uint64(dynamic.Len()))
	// Segment and section contents.
	b.Write(interpBytes)
	b.Write(dynstr.Bytes())
	b.Write(dynamic.Bytes())
	b.Write(shstrtab)
	// Section headers.
	shdr := func(name uint32, typ elf.SectionType, offset, size uint64, link uint32, entsize uint64) {
		binary.Write(&b, le, name)
		binary.Write(&b, le, uint32(typ))
		binary.Write(&b, le, uint64(0)) // flags
		binary.Write(&b, le, offset)    // addr
		binary.Write(&b, le, offset)    // offset
		binary.Write(&b, le, size)
		binary.Write(&b, le, link)
		binary.Write(&b, le, uint32(0)) // info
		binary.Write(&b, le, uint64(1)) // addralign
		binary.Write(&b, le, entsize)
	}
	shdr(0, elf.SHT_NULL, 0, 0, 0, 0)
	shdr(1, elf.SHT_STRTAB, dynstrOff, uint64(dynstr.Len()), 0, 0)   // .dynstr
	shdr(9, elf.SHT_DYNAMIC, dynamicOff, uint64(dynamic.Len()), 1, 16) // .dynamic -> .dynstr
	shdr(18, elf.SHT_STRTAB, shstrOff, uint64(len(shstrtab)), 0, 0)  // .shstrtab
	return b.Bytes()
}

// image is a fixture root: files by image path, symlinks by image path
// to target.
type image struct {
	files map[string][]byte
	links map[string]string
}

func (im image) write(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for p, b := range im.files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, b, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for p, target := range im.links {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, full); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestFixtureIsReadable(t *testing.T) {
	root := image{files: map[string][]byte{
		"/bin/app": minimalELF(elf.EM_X86_64, "/lib64/ld-linux-x86-64.so.2", []string{"libc.so.6", "libfoo.so.1"}, "$ORIGIN/../lib"),
	}}.write(t)
	f, err := elf.Open(filepath.Join(root, "bin", "app"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if got := interpreter(f); got != "/lib64/ld-linux-x86-64.so.2" {
		t.Errorf("interpreter = %q", got)
	}
	libs, err := f.ImportedLibraries()
	if err != nil || strings.Join(libs, ",") != "libc.so.6,libfoo.so.1" {
		t.Errorf("ImportedLibraries = %v, %v", libs, err)
	}
	if got := runpath(f, "/bin"); strings.Join(got, ",") != "/lib" {
		t.Errorf("runpath = %v", got)
	}
}

func TestCheck(t *testing.T) {
	x86 := elf.EM_X86_64
	lib := func(m elf.Machine) []byte { return minimalELF(m, "", nil, "") }
	cases := []struct {
		name   string
		image  image
		allows []allow
		wantOK bool
		want   []string // substrings of the report
	}{
		{
			name: "resolves through default dirs, a symlinked interpreter and RUNPATH $ORIGIN",
			image: image{
				files: map[string][]byte{
					"/opt/app/bin/app":                      minimalELF(x86, "/lib64/ld-linux-x86-64.so.2", []string{"libc.so.6", "libapp.so.1"}, "$ORIGIN/../lib"),
					"/usr/lib/x86_64-linux-gnu/libc.so.6":   lib(x86),
					"/usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2": lib(x86),
					"/opt/app/lib/libapp.so.1":              lib(x86),
					"/usr/share/zoneinfo/UTC":               []byte("TZif2 not an ELF"),
				},
				links: map[string]string{"/lib64/ld-linux-x86-64.so.2": "/usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2"},
			},
			wantOK: true,
		},
		{
			name: "a missing library and a missing interpreter are named",
			image: image{files: map[string][]byte{
				"/registry": minimalELF(x86, "/lib64/ld-linux-x86-64.so.2", []string{"libc.so.6"}, ""),
			}},
			wantOK: false,
			want:   []string{"MISSING  /registry cannot load: interpreter /lib64/ld-linux-x86-64.so.2, libc.so.6"},
		},
		{
			name: "a fully static binary needs nothing",
			image: image{files: map[string][]byte{
				"/registry": minimalELF(x86, "", nil, ""),
			}},
			wantOK: true,
		},
		{
			name: "a library for another machine does not count",
			image: image{files: map[string][]byte{
				"/usr/bin/tool":     minimalELF(x86, "", []string{"libz.so.1"}, ""),
				"/usr/lib/libz.so.1": lib(elf.EM_AARCH64),
			}},
			wantOK: false,
			want:   []string{"MISSING  /usr/bin/tool cannot load: libz.so.1"},
		},
		{
			name: "ld.so.conf.d adds search directories",
			image: image{files: map[string][]byte{
				"/usr/bin/tool":           minimalELF(x86, "", []string{"libextra.so.1"}, ""),
				"/opt/extra/libextra.so.1": lib(x86),
				"/etc/ld.so.conf":          []byte("include /etc/ld.so.conf.d/*.conf\n"),
				"/etc/ld.so.conf.d/x.conf": []byte("# extra\n/opt/extra\n"),
			}},
			wantOK: true,
		},
		{
			name: "an allowed file is reported, not failed",
			image: image{files: map[string][]byte{
				"/usr/lib64/python3.12/lib-dynload/nis.cpython-312-x86_64-linux-gnu.so": minimalELF(x86, "", []string{"libnsl.so.3"}, ""),
			}},
			allows: []allow{{glob: "/usr/lib64/python3.12/lib-dynload/nis.cpython-312-*-linux-gnu.so", reason: "nis chain not shipped"}},
			wantOK: true,
			want:   []string{"allowed  /usr/lib64/python3.12/lib-dynload/nis.cpython-312-x86_64-linux-gnu.so: libnsl.so.3 (nis chain not shipped)"},
		},
		{
			name: "an allow that silences nothing is stale",
			image: image{files: map[string][]byte{
				"/usr/bin/tool": minimalELF(x86, "", nil, ""),
			}},
			allows: []allow{{glob: "/usr/bin/tool", reason: "no longer true"}},
			wantOK: false,
			want:   []string{`STALE    --allow "/usr/bin/tool" silences nothing`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			report, ok, err := check(c.image.write(t), c.allows)
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			if ok != c.wantOK {
				t.Errorf("ok = %v, want %v; report:\n%s", ok, c.wantOK, report)
			}
			for _, w := range c.want {
				if !strings.Contains(report, w) {
					t.Errorf("report lacks %q:\n%s", w, report)
				}
			}
		})
	}
}

func TestAllowFlag(t *testing.T) {
	var a allowFlag
	if err := a.Set("/usr/bin/x=because"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"/usr/bin/x", "=reason", "[=reason"} {
		if err := a.Set(bad); err == nil {
			t.Errorf("Set(%q) accepted", bad)
		}
	}
}
