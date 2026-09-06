// elfneeds: every ELF in an image root resolves what it loads, the way
// the dynamic loader will.
//
// Three defects in one week were one defect: an ELF in the image needed
// something the image did not carry. libb2 needed libgomp; libstdc++
// needed libgcc_s, which static's closure had been supplying by accident;
// the registry binary needed ld-linux and glibc after static stopped
// shipping them. Each was found by a different behavioural test, late.
// This tool asks the artifact: for every ELF under --root it reads
// PT_INTERP, DT_NEEDED and DT_RUNPATH/DT_RPATH with the standard library's
// debug/elf, and resolves each against the root as ld.so would — RUNPATH
// with $ORIGIN expanded, then the loader's default directories and
// /etc/ld.so.conf. A library found for another machine does not count.
//
// It sees link-time needs only. A library opened by name at runtime, as
// node's libatomic is, needs a test that runs the binary.
//
// An unresolved need is a failure unless --allow names the file with a
// reason, and an --allow that silences nothing is a failure too, so the
// list of what an image knowingly cannot load stays true.
package main

import (
	"debug/elf"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// defaultDirs are where ld.so looks after RUNPATH and ld.so.conf: glibc's
// system directories on both architectures the images are built for,
// Debian's multiarch layout beside RHEL's lib64.
var defaultDirs = []string{
	"/lib", "/usr/lib", "/lib64", "/usr/lib64",
	"/lib/x86_64-linux-gnu", "/usr/lib/x86_64-linux-gnu",
	"/lib/aarch64-linux-gnu", "/usr/lib/aarch64-linux-gnu",
}

type allowFlag []allow

type allow struct {
	glob, reason string
}

func (a *allowFlag) String() string { return fmt.Sprint(*a) }

func (a *allowFlag) Set(v string) error {
	glob, reason, ok := strings.Cut(v, "=")
	if !ok || glob == "" || reason == "" {
		return fmt.Errorf("--allow wants <path glob>=<reason>, got %q", v)
	}
	if _, err := path.Match(glob, ""); err != nil {
		return fmt.Errorf("--allow %q: %w", glob, err)
	}
	*a = append(*a, allow{glob: glob, reason: reason})
	return nil
}

func main() {
	root := flag.String("root", "", "directory holding the image's filesystem, every layer extracted in order")
	var allows allowFlag
	flag.Var(&allows, "allow", "<path glob>=<reason>: a file whose unresolved needs are known and accepted; repeatable")
	flag.Parse()
	if *root == "" {
		fmt.Fprintln(os.Stderr, "elfneeds: --root is required")
		os.Exit(2)
	}
	report, ok, err := check(*root, allows)
	if err != nil {
		fmt.Fprintln(os.Stderr, "elfneeds:", err)
		os.Exit(1)
	}
	fmt.Print(report)
	if !ok {
		os.Exit(1)
	}
}

// unresolved is one ELF and what it could not find.
type unresolved struct {
	file    string
	missing []string
}

// check walks root and returns a report and whether the image passes.
func check(root string, allows []allow) (string, bool, error) {
	searchDirs := append(ldSoConfDirs(root), defaultDirs...)
	var failures []unresolved
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		imagePath := "/" + filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator)))
		if strings.HasPrefix(imagePath, "/usr/lib/debug/") {
			return nil
		}
		missing, err := needsOf(root, imagePath, searchDirs)
		if err != nil {
			return fmt.Errorf("%s: %w", imagePath, err)
		}
		if len(missing) > 0 {
			failures = append(failures, unresolved{file: imagePath, missing: missing})
		}
		return nil
	})
	if err != nil {
		return "", false, err
	}
	sort.Slice(failures, func(i, j int) bool { return failures[i].file < failures[j].file })

	var b strings.Builder
	ok := true
	used := map[string]bool{}
	for _, f := range failures {
		reason := ""
		for _, a := range allows {
			if m, _ := path.Match(a.glob, f.file); m {
				reason = a.reason
				used[a.glob] = true
				break
			}
		}
		if reason != "" {
			fmt.Fprintf(&b, "allowed  %s: %s (%s)\n", f.file, strings.Join(f.missing, ", "), reason)
			continue
		}
		ok = false
		fmt.Fprintf(&b, "MISSING  %s cannot load: %s\n", f.file, strings.Join(f.missing, ", "))
	}
	for _, a := range allows {
		if !used[a.glob] {
			ok = false
			fmt.Fprintf(&b, "STALE    --allow %q silences nothing; remove it\n", a.glob)
		}
	}
	return b.String(), ok, nil
}

// needsOf returns what the ELF at imagePath needs and cannot find, or
// nothing if the file is not an ELF or is fully static.
func needsOf(root, imagePath string, searchDirs []string) ([]string, error) {
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(imagePath)))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	magic := make([]byte, 4)
	if n, _ := io.ReadFull(f, magic); n < 4 || string(magic) != elf.ELFMAG {
		return nil, nil
	}
	ef, err := elf.NewFile(f)
	if err != nil {
		return nil, fmt.Errorf("parse ELF: %w", err)
	}
	defer ef.Close()

	var missing []string
	if interp := interpreter(ef); interp != "" {
		if _, ok := fileInRoot(root, interp); !ok {
			missing = append(missing, "interpreter "+interp)
		}
	}
	needed, err := ef.ImportedLibraries()
	if err != nil {
		return nil, fmt.Errorf("DT_NEEDED: %w", err)
	}
	if len(needed) == 0 {
		return missing, nil
	}
	dirs := append(runpath(ef, path.Dir(imagePath)), searchDirs...)
	for _, soname := range needed {
		if !resolves(root, soname, dirs, ef.Machine) {
			missing = append(missing, soname)
		}
	}
	return missing, nil
}

func interpreter(f *elf.File) string {
	for _, p := range f.Progs {
		if p.Type != elf.PT_INTERP {
			continue
		}
		b, err := io.ReadAll(p.Open())
		if err != nil {
			return ""
		}
		return strings.TrimRight(string(b), "\x00")
	}
	return ""
}

// runpath is DT_RUNPATH, or DT_RPATH when there is none, with $ORIGIN
// expanded to the ELF's own directory in the image.
func runpath(f *elf.File, origin string) []string {
	entries, _ := f.DynString(elf.DT_RUNPATH)
	if len(entries) == 0 {
		entries, _ = f.DynString(elf.DT_RPATH)
	}
	var dirs []string
	for _, e := range entries {
		for _, dir := range strings.Split(e, ":") {
			if dir == "" {
				continue
			}
			dir = strings.ReplaceAll(dir, "${ORIGIN}", origin)
			dir = strings.ReplaceAll(dir, "$ORIGIN", origin)
			dirs = append(dirs, path.Clean(dir))
		}
	}
	return dirs
}

// resolves reports whether soname is a loadable library for machine in
// one of dirs, in order.
func resolves(root, soname string, dirs []string, machine elf.Machine) bool {
	for _, dir := range dirs {
		real, ok := fileInRoot(root, path.Join(dir, soname))
		if !ok {
			continue
		}
		if machineOf(real) == machine {
			return true
		}
	}
	return false
}

func machineOf(realPath string) elf.Machine {
	ef, err := elf.Open(realPath)
	if err != nil {
		return elf.EM_NONE
	}
	defer ef.Close()
	return ef.Machine
}

// fileInRoot resolves imagePath inside root, following symlinks as the
// image's own filesystem would — an absolute target is absolute within
// root — and returns the real path if it is a regular file.
func fileInRoot(root, imagePath string) (string, bool) {
	p := path.Clean(imagePath)
	for i := 0; i < 40; i++ {
		real := filepath.Join(root, filepath.FromSlash(p))
		info, err := os.Lstat(real)
		if err != nil {
			return "", false
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return real, info.Mode().IsRegular()
		}
		target, err := os.Readlink(real)
		if err != nil {
			return "", false
		}
		if path.IsAbs(target) {
			p = path.Clean(target)
		} else {
			p = path.Join(path.Dir(p), target)
		}
	}
	return "", false
}

// ldSoConfDirs reads /etc/ld.so.conf and the files it includes, the
// directories the loader searches before its defaults.
func ldSoConfDirs(root string) []string {
	var dirs []string
	var read func(imagePath string, depth int)
	read = func(imagePath string, depth int) {
		if depth > 8 {
			return
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(imagePath)))
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if pattern, ok := strings.CutPrefix(line, "include "); ok {
				matches, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(strings.TrimSpace(pattern))))
				sort.Strings(matches)
				for _, m := range matches {
					read("/"+filepath.ToSlash(strings.TrimPrefix(m, root+string(filepath.Separator))), depth+1)
				}
				continue
			}
			dirs = append(dirs, path.Clean(line))
		}
	}
	read("/etc/ld.so.conf", 0)
	return dirs
}
