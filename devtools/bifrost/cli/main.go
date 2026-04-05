package main

import (
	"flag"
	"io"
	"log"
	"os"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
	"github.com/arkeros/senku/devtools/bifrost/cloudrun"
	"github.com/arkeros/senku/devtools/bifrost/k8s"
	"github.com/arkeros/senku/devtools/bifrost/terraform"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "render" {
		log.Fatalf("usage: bifrost render <cloudrun|k8s|terraform> -f <workload.{yaml,json}>")
	}

	target := os.Args[2]
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	var inputPath string
	fs.StringVar(&inputPath, "f", "", "Path to the service spec YAML or JSON")
	if err := fs.Parse(os.Args[3:]); err != nil {
		log.Fatal(err)
	}
	if inputPath == "" {
		log.Fatal("-f is required")
	}

	reader, closeFn, err := inputReader(inputPath)
	if err != nil {
		log.Fatal(err)
	}
	if closeFn != nil {
		defer closeFn()
	}

	spec, err := bifrost.Parse(reader)
	if err != nil {
		log.Fatal(err)
	}

	var out []byte
	switch target {
	case "cloudrun":
		out, err = cloudrun.Render(spec)
	case "k8s":
		out, err = k8s.Render(spec)
	case "terraform":
		out, err = terraform.Render(spec)
	default:
		log.Fatalf("unsupported render target %q", target)
	}
	if err != nil {
		log.Fatal(err)
	}

	if _, err := os.Stdout.Write(out); err != nil {
		log.Fatal(err)
	}
}

func inputReader(path string) (io.Reader, func() error, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}
