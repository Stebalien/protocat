package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	ggio "github.com/gogo/protobuf/io"
	jsonpb "github.com/golang/protobuf/jsonpb"
	proto "github.com/golang/protobuf/proto"
	protoparse "github.com/jhump/protoreflect/desc/protoparse"
	protodynamic "github.com/jhump/protoreflect/dynamic"
)

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

func loadMessage(path []string, name string) (proto.Message, error) {
	if len(path) == 0 {
		path = append(gopath(), ".", "/")
	}
	parser := protoparse.Parser{
		ImportPaths: path,
	}
	typeOffset := strings.LastIndexByte(name, '.')
	if typeOffset < 0 {
		return nil, fmt.Errorf("no protobuf message name in %s", name)
	}
	messageName := name[typeOffset+1:]
	fileName := name[:typeOffset] + ".proto"

	descriptors, err := parser.ParseFiles(fileName)
	if err != nil {
		return nil, err
	}
	d := descriptors[0]
	md := d.FindMessage(fmt.Sprintf("%s.%s", d.GetPackage(), messageName))
	if md == nil {
		return nil, fmt.Errorf("message %s not defined in %s", messageName, fileName)
	}
	return protodynamic.NewMessage(md), nil
}

func main() {
	var flags flag.FlagSet
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: protocat [-d] [-l] [-m MAX_SIZE] TYPE [PATH ...]")
		flags.PrintDefaults()
	}
	decode := flags.Bool("d", false, "decode (default encode)")
	delimited := flags.Bool("l", false, "length delimited (varint)")
	maxSize := flags.Int("m", 8*1024, "max message size (in KiB)")
	switch err := flags.Parse(os.Args[1:]); err {
	case nil:
	case flag.ErrHelp:
		return
	default:
		os.Exit(2)
	}
	if flags.NArg() < 1 {
		fmt.Fprintln(flags.Output(), "protobuf message type required")
		flags.Usage()
		os.Exit(2)
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
