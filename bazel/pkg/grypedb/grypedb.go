package grypedb

import (
	"fmt"
	"os"

	"github.com/bazelbuild/buildtools/build"
	"github.com/bazelbuild/buildtools/edit/bzlmod"
)

// UpdateModuleFile updates the grype database URL and SHA256 in a MODULE.bazel file.
func UpdateModuleFile(path, newURL, newSHA256 string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	ast, err := build.ParseModule(path, content)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", path, err)
	}

	if err := updateGrypeDatabaseCall(ast, newURL, newSHA256); err != nil {
		return err
	}

	formatted, err := formatModule(ast)
	if err != nil {
		return fmt.Errorf("failed to format %s: %w", path, err)
	}

	if err := os.WriteFile(path, formatted, 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}

	return nil
}

func updateGrypeDatabaseCall(file *build.File, url, sha256sum string) error {
	proxyName, err := findGrypeExtensionProxy(file)
	if err != nil {
		return err
	}

	for _, stmt := range file.Stmt {
		call, ok := stmt.(*build.CallExpr)
		if !ok {
			continue
		}

		dotExpr, ok := call.X.(*build.DotExpr)
		if !ok {
			continue
		}

		xIdent, ok := dotExpr.X.(*build.Ident)
		if !ok || xIdent.Name != proxyName || dotExpr.Name != "database" {
			continue
		}

		updateCallArguments(call, url, sha256sum)
		return nil
	}

	return fmt.Errorf("could not find %s.database() call in %s", proxyName, file.Path)
}

func findGrypeExtensionProxy(file *build.File) (string, error) {
	proxies := bzlmod.Proxies(file, "@grype.bzl//grype:extensions.bzl", "grype_database", false)
	if len(proxies) > 0 {
		return proxies[0], nil
	}

	proxies = bzlmod.Proxies(file, "@grype.bzl//grype:extensions.bzl", "grype_database", true)
	if len(proxies) > 0 {
		return proxies[0], nil
	}

	return "", fmt.Errorf("could not find use_extension call for grype extension in %s", file.Path)
}

func updateCallArguments(call *build.CallExpr, url, sha256sum string) {
	updatedSha := false
	updatedURL := false

	for _, arg := range call.List {
		assign, ok := arg.(*build.AssignExpr)
		if !ok {
			continue
		}

		lhs, ok := assign.LHS.(*build.Ident)
		if !ok {
			continue
		}

		switch lhs.Name {
		case "sha256":
			assign.RHS = &build.StringExpr{Value: sha256sum}
			updatedSha = true
		case "url":
			assign.RHS = &build.StringExpr{Value: url}
			updatedURL = true
		}
	}

	if !updatedSha {
		call.List = append(call.List, &build.AssignExpr{
			LHS: &build.Ident{Name: "sha256"},
			Op:  "=",
			RHS: &build.StringExpr{Value: sha256sum},
		})
	}

	if !updatedURL {
		call.List = append(call.List, &build.AssignExpr{
			LHS: &build.Ident{Name: "url"},
			Op:  "=",
			RHS: &build.StringExpr{Value: url},
		})
	}
}

func formatModule(f *build.File) ([]byte, error) {
	contents := build.Format(f)

	newF, err := build.ParseModule(f.Path, contents)
	if err != nil {
		return nil, err
	}

	return build.Format(newF), nil
}
