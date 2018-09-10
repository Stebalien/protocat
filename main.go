package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	ggio "github.com/gogo/protobuf/io"
	jsonpb "github.com/golang/protobuf/jsonpb"
	proto "github.com/golang/protobuf/proto"
	protoparse "github.com/jhump/protoreflect/desc/protoparse"
	protodynamic "github.com/jhump/protoreflect/dynamic"
)

var Info = log.New(ioutil.Discard, "I: ", 0)
var Error = log.New(os.Stderr, "E: ", 0)

func gopath() []string {
	path := filepath.SplitList(os.Getenv("GOPATH"))
	if len(path) == 0 {
		home := os.Getenv("HOME")
		if home != "" {
			path = []string{home + "/go"}
		}
	}
	for i := range path {
		path[i] += "/src"
	}
	return path
}

func loadMessage(files []string, messageName string) (proto.Message, error) {
	parser := protoparse.Parser{
		ImportPaths: append(gopath(), "."),
	}
	descriptors, err := parser.ParseFiles(files...)
	if err != nil {
		Error.Fatal(err)
	}

	for _, d := range descriptors {
		if md := d.FindMessage(messageName); md != nil {
			return protodynamic.NewMessage(md), nil
		}
	}
	return nil, fmt.Errorf("message %s not defined", messageName)
}

func main() {
	var flags flag.FlagSet
	decode := flags.Bool("d", false, "decode instead of encoding")
	delimited := flags.Bool("l", false, "length delimited (varint)")
	verbose := flags.Bool("v", false, "print status messages")
	maxSize := flags.Int("m", 8*1024, "max message size (in KiB)")
	switch err := flags.Parse(os.Args[1:]); err {
	case nil:
	case flag.ErrHelp:
		return
	default:
		os.Exit(2)
	}
	if *verbose {
		Info.SetOutput(os.Stderr)
	}
	if flags.NArg() < 2 {
		Error.Fatal("invalid arguments")
	}
	message, err := loadMessage(flags.Args()[1:], flags.Arg(0))
	if err != nil {
		Error.Fatal(err)
	}

	if *decode {
		marshaler := jsonpb.Marshaler{
			Indent:   "  ",
			OrigName: true,
		}
		var reader ggio.Reader
		if *delimited {
			reader = ggio.NewDelimitedReader(os.Stdin, *maxSize)
		} else {
			reader = ggio.NewFullReader(os.Stdin, *maxSize)
		}
		for {
			switch err := reader.ReadMsg(message); err {
			case nil:
			case io.EOF:
				return
			default:
				Error.Fatal(err)
			}
			if err := marshaler.Marshal(os.Stdout, message); err != nil {
				Error.Fatal(err)
			}
			message.Reset()
		}
	} else {
		decoder := json.NewDecoder(os.Stdin)
		var writer ggio.Writer
		if *delimited {
			writer = ggio.NewDelimitedWriter(os.Stdout)
		} else {
			writer = ggio.NewFullWriter(os.Stdout)
		}
		for {
			switch err := jsonpb.UnmarshalNext(decoder, message); err {
			case nil:
			case io.EOF:
				return
			default:
				Error.Fatal(err)
			}
			if err := writer.WriteMsg(message); err != nil {
				Error.Fatal(err)
			}
			message.Reset()
		}
	}
}
